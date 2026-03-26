package dev.allyourbase

import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.delay
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import kotlin.time.Duration
import kotlin.time.Duration.Companion.seconds

data class AYBConfiguration(
    val baseURL: String,
    val apiKey: String? = null,
    val timeout: Duration = 30.seconds,
    val maxRetries: Int = 0,
    val retryDelay: Duration = Duration.ZERO,
)

enum class AuthStateEvent {
    SIGNED_IN,
    SIGNED_OUT,
    TOKEN_REFRESHED,
}

data class AuthSession(
    val token: String,
    val refreshToken: String,
)

class AYBClient(
    baseURL: String,
    apiKey: String? = null,
    transport: HttpTransport? = null,
    sseTransport: SseTransport? = null,
    wsTransport: WebSocketTransport? = null,
    tokenStore: TokenStore? = null,
    timeout: Duration = 30.seconds,
    maxRetries: Int = 0,
    retryDelay: Duration = Duration.ZERO,
) {
    val configuration: AYBConfiguration
    val transport: HttpTransport
    val sseTransport: SseTransport
    val wsTransport: WebSocketTransport
    val tokenStore: TokenStore

    val token: String?
        get() = tokenStore.accessToken()

    val refreshToken: String?
        get() = tokenStore.refreshToken()

    private val json = Json { ignoreUnknownKeys = true }
    private val requestBuilder: RequestBuilder

    private val listenersLock = Any()
    private val listeners = LinkedHashMap<String, (AuthStateEvent, AuthSession?) -> Unit>()

    val auth: AuthClient by lazy { AuthClient(this) }
    val records: RecordsClient by lazy { RecordsClient(this) }
    val storage: StorageClient by lazy { StorageClient(this) }
    val realtime: RealtimeClient by lazy { RealtimeClient(this) }

    init {
        val normalizedBaseURL = baseURL.trimEnd('/')
        configuration = AYBConfiguration(
            baseURL = normalizedBaseURL,
            apiKey = apiKey,
            timeout = timeout,
            maxRetries = maxRetries,
            retryDelay = retryDelay,
        )

        this.tokenStore = tokenStore ?: InMemoryTokenStore()
        if (!apiKey.isNullOrEmpty()) {
            this.tokenStore.save(apiKey, null)
        }

        this.transport = transport ?: KtorHttpTransport(timeout = timeout)
        this.sseTransport = sseTransport ?: OkHttpSseTransport()
        this.wsTransport = wsTransport ?: OkHttpWebSocketTransport()
        this.requestBuilder = RequestBuilder(normalizedBaseURL)
    }

    suspend fun <T> request(
        path: String,
        method: HttpMethod = HttpMethod.GET,
        query: Map<String, String> = emptyMap(),
        queryItems: List<Pair<String, String>> = emptyList(),
        headers: Map<String, String> = emptyMap(),
        body: JsonElement? = null,
        rawBody: ByteArray? = null,
        rawContentType: String? = null,
        skipAuth: Boolean = false,
        decode: (JsonElement?) -> T,
    ): T {
        val bearerToken = if (skipAuth) null else tokenStore.accessToken()
        val request = requestBuilder.buildRequest(
            path = path,
            method = method,
            query = query,
            queryItems = queryItems,
            headers = headers,
            body = body,
            rawBody = rawBody,
            rawContentType = rawContentType,
            bearerToken = bearerToken,
        )

        val response = sendWithRetries(request)

        if (response.statusCode !in 200..299) {
            throw AYBException.from(response)
        }

        val payload = if (response.statusCode == 204 || response.body.isEmpty()) {
            null
        } else {
            json.parseToJsonElement(response.body.decodeToString())
        }

        return decode(payload)
    }

    suspend fun sendWithRetries(request: HttpRequest): HttpResponse {
        var attempt = 0
        while (true) {
            try {
                return transport.send(request)
            } catch (error: Throwable) {
                if (error is CancellationException) {
                    throw error
                }

                if (attempt >= configuration.maxRetries) {
                    throw error
                }

                attempt += 1
                if (configuration.retryDelay > Duration.ZERO) {
                    delay(configuration.retryDelay.inWholeMilliseconds)
                }
            }
        }
    }

    fun setTokens(token: String, refreshToken: String) {
        tokenStore.save(token, refreshToken)
    }

    fun clearTokens() {
        tokenStore.clear()
    }

    fun setApiKey(apiKey: String) {
        tokenStore.save(apiKey, null)
    }

    fun clearApiKey() {
        tokenStore.clear()
    }

    fun onAuthStateChange(listener: (AuthStateEvent, AuthSession?) -> Unit): () -> Unit {
        val id = java.util.UUID.randomUUID().toString()
        synchronized(listenersLock) {
            listeners[id] = listener
        }

        return {
            synchronized(listenersLock) {
                listeners.remove(id)
            }
        }
    }

    internal fun emitAuthState(event: AuthStateEvent) {
        val session = tokenStore.accessToken()?.let { access ->
            tokenStore.refreshToken()?.let { refresh ->
                AuthSession(token = access, refreshToken = refresh)
            }
        }

        val snapshot = synchronized(listenersLock) {
            listeners.values.toList()
        }

        snapshot.forEach { listener ->
            listener(event, session)
        }
    }
}
