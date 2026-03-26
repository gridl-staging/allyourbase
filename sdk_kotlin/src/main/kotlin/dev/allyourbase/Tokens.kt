package dev.allyourbase

interface TokenStore {
    fun accessToken(): String?
    fun refreshToken(): String?
    fun save(accessToken: String?, refreshToken: String?)
    fun clear()
}

class InMemoryTokenStore(
    private var access: String? = null,
    private var refresh: String? = null,
) : TokenStore {
    private val lock = Any()

    override fun accessToken(): String? = synchronized(lock) { access }

    override fun refreshToken(): String? = synchronized(lock) { refresh }

    override fun save(accessToken: String?, refreshToken: String?) {
        synchronized(lock) {
            access = accessToken
            refresh = refreshToken
        }
    }

    override fun clear() {
        synchronized(lock) {
            access = null
            refresh = null
        }
    }
}
