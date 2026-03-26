import 'dart:async';
import 'dart:convert';
import 'dart:math' as math;
import 'dart:typed_data';

import 'package:http/http.dart' as http;
import 'package:http_parser/http_parser.dart' show MediaType;

import 'errors.dart';
import 'sse/sse_parser.dart';
import 'types.dart';

typedef AuthStateListener = void Function(String event, AuthSession? session);

class AuthStateEvent {
  static const String signedIn = 'SIGNED_IN';
  static const String signedOut = 'SIGNED_OUT';
  static const String tokenRefreshed = 'TOKEN_REFRESHED';
}

class AuthSession {
  const AuthSession({
    required this.token,
    required this.refreshToken,
  });

  final String token;
  final String refreshToken;
}

/// TODO: Document RealtimeOptions.
class RealtimeOptions {
  const RealtimeOptions({
    this.maxReconnectAttempts = 5,
    this.reconnectDelays = const <Duration>[
      Duration(milliseconds: 200),
      Duration(milliseconds: 500),
      Duration(seconds: 1),
      Duration(seconds: 2),
      Duration(seconds: 5),
    ],
    this.jitterMax = const Duration(milliseconds: 100),
    this.sleep,
    this.randomDouble,
  }) : assert(maxReconnectAttempts >= 0);

  final int maxReconnectAttempts;
  final List<Duration> reconnectDelays;
  final Duration jitterMax;
  final Future<void> Function(Duration delay)? sleep;
  final double Function()? randomDouble;
}

/// TODO: Document AYBClient.
class AYBClient {
  AYBClient(
    String baseUrl, {
    http.Client? httpClient,
    RealtimeOptions realtimeOptions = const RealtimeOptions(),
  })  : baseUrl = _normalizeBaseUrl(baseUrl),
        _httpClient = httpClient ?? http.Client() {
    auth = AuthClient(this);
    records = RecordsClient(this);
    storage = StorageClient(this);
    realtime = RealtimeClient(this, options: realtimeOptions);
    push = PushClient(this);
  }

  final String baseUrl;
  final http.Client _httpClient;
  final Set<AuthStateListener> _authListeners = <AuthStateListener>{};
  String? _token;
  String? _refreshToken;

  late final AuthClient auth;
  late final RecordsClient records;
  late final StorageClient storage;
  late final RealtimeClient realtime;
  late final PushClient push;

  http.Client get httpClient => _httpClient;
  String? get token => _token;
  String? get refreshToken => _refreshToken;

  void setTokens(String token, String refreshToken) {
    _token = token;
    _refreshToken = refreshToken;
  }

  void clearTokens() {
    _token = null;
    _refreshToken = null;
  }

  void setApiKey(String apiKey) {
    _token = apiKey;
    _refreshToken = null;
  }

  void clearApiKey() {
    clearTokens();
  }

  void setTokensInternal(String token, String refreshToken) {
    setTokens(token, refreshToken);
  }

  void Function() onAuthStateChange(AuthStateListener listener) {
    _authListeners.add(listener);
    return () {
      _authListeners.remove(listener);
    };
  }

  void emitAuthEvent(String event) {
    final session = (_token != null && _refreshToken != null)
        ? AuthSession(token: _token!, refreshToken: _refreshToken!)
        : null;
    // Copy the set to avoid ConcurrentModificationError if a listener
    // adds/removes listeners during the callback.
    final snapshot = List<AuthStateListener>.of(_authListeners);
    for (final listener in snapshot) {
      listener(event, session);
    }
  }

  Future<T> request<T>(
    String path, {
    String method = 'GET',
    Map<String, String>? headers,
    Object? body,
    bool skipAuth = false,
    T Function(Object? value)? decode,
  }) async {
    final requestHeaders = <String, String>{...?headers};
    if (!skipAuth && _token != null) {
      requestHeaders['Authorization'] = 'Bearer $_token';
    }

    final request = http.Request(method, _buildUri(path));
    _applyRequestBody(request, requestHeaders, body);
    request.headers.addAll(requestHeaders);

    final response = await http.Response.fromStream(
      await _httpClient.send(request),
    );

    if (response.statusCode < 200 || response.statusCode >= 300) {
      throw _normalizeHttpError(response);
    }

    if (response.statusCode == 204 || response.bodyBytes.isEmpty) {
      return _nullableCast<T>(null);
    }

    final decodedBody = jsonDecode(utf8.decode(response.bodyBytes));
    if (decode != null) {
      return decode(decodedBody);
    }
    return _nullableCast<T>(decodedBody);
  }

  Future<T> rpc<T>(
    String functionName, {
    Map<String, Object?>? args,
  }) {
    final hasArgs = args != null && args.isNotEmpty;
    return request<T>(
      '/api/rpc/$functionName',
      method: 'POST',
      body: hasArgs ? args : null,
    );
  }

  void close() {
    _httpClient.close();
  }

  Uri _buildUri(String path) {
    final normalizedPath = path.startsWith('/') ? path : '/$path';
    return Uri.parse('$baseUrl$normalizedPath');
  }

  void _applyRequestBody(
    http.Request request,
    Map<String, String> headers,
    Object? body,
  ) {
    if (body == null) {
      return;
    }
    if (body is String) {
      request.body = body;
      return;
    }
    if (body is List<int>) {
      request.bodyBytes = body;
      return;
    }

    if (!_containsHeader(headers, 'Content-Type')) {
      headers['Content-Type'] = 'application/json';
    }
    request.body = jsonEncode(body);
  }

  static bool _containsHeader(Map<String, String> headers, String name) {
    for (final key in headers.keys) {
      if (key.toLowerCase() == name.toLowerCase()) {
        return true;
      }
    }
    return false;
  }

  static T _nullableCast<T>(Object? value) {
    return value as T;
  }

  static String _normalizeBaseUrl(String value) {
    var result = value;
    while (result.endsWith('/')) {
      result = result.substring(0, result.length - 1);
    }
    return result;
  }
}

/// TODO: Document AuthClient.
class AuthClient {
  AuthClient(this.client);

  final AYBClient client;

  /// Register a new user account.
  Future<AuthResponse> register(String email, String password) async {
    final response = await client.request<AuthResponse>(
      '/api/auth/register',
      method: 'POST',
      body: {'email': email, 'password': password},
      decode: (value) => AuthResponse.fromJson(value as JsonMap),
    );
    client.setTokensInternal(response.token, response.refreshToken);
    client.emitAuthEvent(AuthStateEvent.signedIn);
    return response;
  }

  /// Log in with email and password.
  Future<AuthResponse> login(String email, String password) async {
    final response = await client.request<AuthResponse>(
      '/api/auth/login',
      method: 'POST',
      body: {'email': email, 'password': password},
      decode: (value) => AuthResponse.fromJson(value as JsonMap),
    );
    client.setTokensInternal(response.token, response.refreshToken);
    client.emitAuthEvent(AuthStateEvent.signedIn);
    return response;
  }

  /// Get the current authenticated user.
  Future<User> me() {
    return client.request<User>(
      '/api/auth/me',
      decode: (value) => User.fromJson(value as JsonMap),
    );
  }

  /// Refresh the access token using the stored refresh token.
  Future<AuthResponse> refresh() async {
    final refreshToken = _requireRefreshToken();
    final response = await client.request<AuthResponse>(
      '/api/auth/refresh',
      method: 'POST',
      body: {'refreshToken': refreshToken},
      decode: (value) => AuthResponse.fromJson(value as JsonMap),
    );
    client.setTokensInternal(response.token, response.refreshToken);
    client.emitAuthEvent(AuthStateEvent.tokenRefreshed);
    return response;
  }

  /// Log out (revoke the refresh token).
  Future<void> logout() async {
    final refreshToken = _requireRefreshToken();
    await client.request<void>(
      '/api/auth/logout',
      method: 'POST',
      body: {'refreshToken': refreshToken},
    );
    client.clearTokens();
    client.emitAuthEvent(AuthStateEvent.signedOut);
  }

  /// Delete the current authenticated user's account.
  Future<void> deleteAccount() async {
    await client.request<void>('/api/auth/me', method: 'DELETE');
    client.clearTokens();
    client.emitAuthEvent(AuthStateEvent.signedOut);
  }

  /// Request a password reset email.
  Future<void> requestPasswordReset(String email) {
    return client.request<void>(
      '/api/auth/password-reset',
      method: 'POST',
      body: {'email': email},
    );
  }

  /// Confirm a password reset with a token and new password.
  Future<void> confirmPasswordReset(String token, String password) {
    return client.request<void>(
      '/api/auth/password-reset/confirm',
      method: 'POST',
      body: {'token': token, 'password': password},
    );
  }

  /// Verify an email address with a token.
  Future<void> verifyEmail(String token) {
    return client.request<void>(
      '/api/auth/verify',
      method: 'POST',
      body: {'token': token},
    );
  }

  /// Resend the email verification (requires auth).
  Future<void> resendVerification() {
    return client.request<void>(
      '/api/auth/verify/resend',
      method: 'POST',
    );
  }

  /// Parse OAuth tokens from a URI fragment after redirect-based OAuth flow.
  ///
  /// Call this on your callback page/deep link handler when using
  /// redirect-based OAuth (via `signInWithOAuth` with `urlCallback`).
  ///
  /// Returns [AuthResponse] with tokens if found, or `null` if the fragment
  /// is missing or incomplete. The `user` field will have minimal data —
  /// call [me] to fetch the full user profile.
  AuthResponse? handleOAuthRedirect(Uri uri) {
    final fragment = uri.fragment;
    if (fragment.isEmpty) return null;

    late final Map<String, String> params;
    try {
      params = Uri.splitQueryString(fragment);
    } on FormatException {
      return null;
    }
    final token = params['token'];
    final refreshToken = params['refreshToken'];
    if (token == null || refreshToken == null) return null;

    client.setTokensInternal(token, refreshToken);
    client.emitAuthEvent(AuthStateEvent.signedIn);
    return AuthResponse(
      token: token,
      refreshToken: refreshToken,
      user: User(id: '', email: ''),
    );
  }

  String _requireRefreshToken() {
    final refreshToken = client.refreshToken;
    if (refreshToken == null || refreshToken.isEmpty) {
      throw const AYBError(
        400,
        'Missing refresh token',
        code: 'auth/missing-refresh-token',
      );
    }
    return refreshToken;
  }

  /// Sign in with an OAuth provider using redirect + SSE flow.
  ///
  /// 1. Connects to SSE (`/api/realtime?oauth=true`) to get a `clientId`.
  /// 2. Builds the OAuth URL with the `clientId` as state.
  /// 3. Calls [urlCallback] with the OAuth URL — the consumer navigates
  ///    the user there (e.g. via `url_launcher`).
  /// 4. Waits for the SSE `oauth` event with auth tokens.
  /// 5. Stores tokens and emits `SIGNED_IN`.
  ///
  /// For post-redirect token handling (deep links), use [handleOAuthRedirect].
  Future<AuthResponse> signInWithOAuth(
    String provider, {
    required Future<void> Function(String url) urlCallback,
    List<String>? scopes,
  }) async {
    // 1. Connect to SSE to get clientId
    final sseUri = Uri.parse('${client.baseUrl}/api/realtime?oauth=true');
    final sseRequest = http.Request('GET', sseUri);
    final sseResponse = await client.httpClient.send(sseRequest);

    if (sseResponse.statusCode < 200 || sseResponse.statusCode >= 300) {
      throw AYBError(
        sseResponse.statusCode,
        'Failed to connect to OAuth SSE channel',
        code: 'oauth/sse-failed',
      );
    }

    final parser = SseParser(sseResponse.stream);
    final completer = Completer<AuthResponse>();
    var launched = false;

    late final StreamSubscription<SseMessage> subscription;
    subscription = parser.stream.listen(
      (message) {
        if (completer.isCompleted) return;

        if (message.event == 'connected') {
          if (launched) return;
          final rawData = message.data;
          if (rawData == null) {
            completer.completeError(const AYBError(
              500,
              'OAuth connected event missing payload',
              code: 'oauth/invalid-connected-event',
            ));
            subscription.cancel();
            return;
          }

          // Extract clientId and build OAuth URL
          late final Map<String, Object?> data;
          try {
            data = jsonDecode(rawData) as Map<String, Object?>;
          } on FormatException {
            completer.completeError(const AYBError(
              500,
              'OAuth connected event has invalid JSON',
              code: 'oauth/invalid-connected-event',
            ));
            subscription.cancel();
            return;
          } on TypeError {
            completer.completeError(const AYBError(
              500,
              'OAuth connected event has invalid payload',
              code: 'oauth/invalid-connected-event',
            ));
            subscription.cancel();
            return;
          }
          final clientId = data['clientId'];
          if (clientId is! String || clientId.isEmpty) {
            completer.completeError(const AYBError(
              500,
              'OAuth connected event missing clientId',
              code: 'oauth/invalid-connected-event',
            ));
            subscription.cancel();
            return;
          }

          var oauthUrl =
              '${client.baseUrl}/api/auth/oauth/$provider?state=$clientId';
          if (scopes != null && scopes.isNotEmpty) {
            oauthUrl += '&scopes=${Uri.encodeComponent(scopes.join(","))}';
          }

          // Call the consumer's URL callback
          launched = true;
          Future<void>.sync(() => urlCallback(oauthUrl)).catchError(
            (Object error) {
              if (!completer.isCompleted) {
                completer.completeError(error);
                subscription.cancel();
              }
            },
          );
        } else if (message.event == 'oauth') {
          final rawData = message.data;
          if (rawData == null) {
            completer.completeError(const AYBError(
              500,
              'OAuth event missing payload',
              code: 'oauth/invalid-oauth-event',
            ));
            subscription.cancel();
            return;
          }

          late final Map<String, Object?> data;
          try {
            data = jsonDecode(rawData) as Map<String, Object?>;
          } on FormatException {
            completer.completeError(const AYBError(
              500,
              'OAuth event has invalid JSON',
              code: 'oauth/invalid-oauth-event',
            ));
            subscription.cancel();
            return;
          } on TypeError {
            completer.completeError(const AYBError(
              500,
              'OAuth event has invalid payload',
              code: 'oauth/invalid-oauth-event',
            ));
            subscription.cancel();
            return;
          }

          final error = data['error'];
          if (error is String && error.isNotEmpty) {
            completer.completeError(AYBError(
              401,
              error,
              code: 'oauth/provider-error',
            ));
            subscription.cancel();
            return;
          }

          final token = data['token'] as String?;
          final refreshToken = data['refreshToken'] as String?;
          if (token == null || refreshToken == null) {
            completer.completeError(const AYBError(
              500,
              'OAuth response missing tokens',
              code: 'oauth/missing-tokens',
            ));
            subscription.cancel();
            return;
          }

          client.setTokensInternal(token, refreshToken);
          client.emitAuthEvent(AuthStateEvent.signedIn);

          completer.complete(AuthResponse(
            token: token,
            refreshToken: refreshToken,
            user: User(id: '', email: ''),
          ));
          subscription.cancel();
        }
      },
      onError: (Object error) {
        if (!completer.isCompleted) {
          completer.completeError(error);
        }
        subscription.cancel();
      },
      onDone: () {
        if (!completer.isCompleted) {
          completer.completeError(const AYBError(
            503,
            'SSE connection closed before OAuth completed',
            code: 'oauth/sse-closed',
          ));
        }
      },
    );

    return completer.future;
  }
}

/// TODO: Document RecordsClient.
class RecordsClient {
  RecordsClient(this.client);

  final AYBClient client;

  /// List records in a collection with optional filtering, sorting, and pagination.
  Future<ListResponse<JsonMap>> list(
    String collection, {
    ListParams? params,
  }) {
    final queryMap = params?.toQueryMap() ?? const <String, String>{};
    final uri = _buildCollectionUri(collection, queryMap);
    return client.request<ListResponse<JsonMap>>(
      uri,
      decode: (value) => ListResponse.fromJson(
        value as JsonMap,
        decodeItem: (item) => item as JsonMap,
      ),
    );
  }

  /// Get a single record by primary key.
  Future<JsonMap> get(
    String collection,
    String id, {
    GetParams? params,
  }) {
    final queryMap = params?.toQueryMap() ?? const <String, String>{};
    final uri = _buildCollectionUri(collection, queryMap, id: id);
    return client.request<JsonMap>(uri);
  }

  /// Create a new record.
  Future<JsonMap> create(
    String collection,
    Map<String, Object?> data,
  ) {
    return client.request<JsonMap>(
      '/api/collections/$collection',
      method: 'POST',
      body: data,
    );
  }

  /// Update an existing record (partial update).
  Future<JsonMap> update(
    String collection,
    String id,
    Map<String, Object?> data,
  ) {
    return client.request<JsonMap>(
      '/api/collections/$collection/$id',
      method: 'PATCH',
      body: data,
    );
  }

  /// Delete a record by primary key.
  Future<void> delete(String collection, String id) {
    return client.request<void>(
      '/api/collections/$collection/$id',
      method: 'DELETE',
    );
  }

  /// Execute multiple operations in a single atomic transaction.
  Future<List<BatchResult<JsonMap>>> batch(
    String collection,
    List<BatchOperation> operations,
  ) {
    return client.request<List<BatchResult<JsonMap>>>(
      '/api/collections/$collection/batch',
      method: 'POST',
      body: {'operations': operations.map((op) => op.toJson()).toList()},
      decode: (value) {
        final list = value as List<Object?>;
        return list
            .map((item) => BatchResult<JsonMap>.fromJson(
                  item as JsonMap,
                  decodeBody: (body) => body as JsonMap,
                ))
            .toList(growable: false);
      },
    );
  }

  String _buildCollectionUri(
    String collection,
    Map<String, String> queryMap, {
    String? id,
  }) {
    final pathSegment = id != null
        ? '/api/collections/$collection/$id'
        : '/api/collections/$collection';
    if (queryMap.isEmpty) return pathSegment;
    return Uri(path: pathSegment, queryParameters: queryMap).toString();
  }
}

/// TODO: Document StorageClient.
class StorageClient {
  StorageClient(this.client);

  final AYBClient client;

  /// Upload a file to a storage bucket.
  ///
  /// [bytes] is the file content as raw bytes.
  /// [name] is the filename to use in the bucket.
  /// [contentType] is an optional MIME type (e.g. `'image/png'`). Defaults to
  /// `application/octet-stream` if not specified.
  Future<StorageObject> upload(
    String bucket,
    Uint8List bytes,
    String name, {
    String? contentType,
  }) async {
    final uri = Uri.parse('${client.baseUrl}/api/storage/$bucket');
    final request = http.MultipartRequest('POST', uri);

    if (client.token != null) {
      request.headers['Authorization'] = 'Bearer ${client.token}';
    }

    request.files.add(http.MultipartFile.fromBytes(
      'file',
      bytes,
      filename: name,
      contentType: contentType != null ? MediaType.parse(contentType) : null,
    ));

    final streamedResponse = await client.httpClient.send(request);
    final response = await http.Response.fromStream(streamedResponse);

    if (response.statusCode < 200 || response.statusCode >= 300) {
      throw _normalizeHttpError(response);
    }

    final body = jsonDecode(utf8.decode(response.bodyBytes)) as JsonMap;
    return StorageObject.fromJson(body);
  }

  /// Build a download URL for a file (synchronous, no HTTP call).
  String downloadUrl(String bucket, String name) {
    return '${client.baseUrl}/api/storage/$bucket/$name';
  }

  /// Delete a file from a storage bucket.
  Future<void> delete(String bucket, String name) {
    return client.request<void>(
      '/api/storage/$bucket/$name',
      method: 'DELETE',
    );
  }

  /// List files in a storage bucket.
  Future<StorageListResponse> list(
    String bucket, {
    String? prefix,
    int? limit,
    int? offset,
  }) {
    final queryMap = <String, String>{};
    if (prefix != null) queryMap['prefix'] = prefix;
    if (limit != null) queryMap['limit'] = limit.toString();
    if (offset != null) queryMap['offset'] = offset.toString();

    final path = queryMap.isEmpty
        ? '/api/storage/$bucket'
        : Uri(
            path: '/api/storage/$bucket',
            queryParameters: queryMap,
          ).toString();

    return client.request<StorageListResponse>(
      path,
      decode: (value) => StorageListResponse.fromJson(value as JsonMap),
    );
  }

  /// Get a signed URL for time-limited access to a file.
  Future<String> getSignedUrl(
    String bucket,
    String name, {
    int expiresIn = 3600,
  }) async {
    final result = await client.request<JsonMap>(
      '/api/storage/$bucket/$name/sign',
      method: 'POST',
      body: {'expiresIn': expiresIn},
    );
    return result['url'] as String;
  }
}

/// Response from [StorageClient.list].
class StorageListResponse {
  const StorageListResponse({
    required this.items,
    required this.totalItems,
  });

  final List<StorageObject> items;
  final int totalItems;

  factory StorageListResponse.fromJson(JsonMap json) {
    final rawItems = json['items'];
    final List<StorageObject> items;
    if (rawItems is List) {
      items = rawItems
          .map((item) => StorageObject.fromJson(item as JsonMap))
          .toList(growable: false);
    } else {
      items = const <StorageObject>[];
    }
    final totalItems = json['totalItems'];
    return StorageListResponse(
      items: items,
      totalItems: totalItems is int ? totalItems : items.length,
    );
  }
}

/// TODO: Document RealtimeClient.
class RealtimeClient {
  RealtimeClient(
    this.client, {
    RealtimeOptions options = const RealtimeOptions(),
  })  : _options = options,
        _random = math.Random();

  final AYBClient client;
  final RealtimeOptions _options;
  final math.Random _random;

  /// Subscribe to realtime events for the given tables.
  ///
  /// Connects to the SSE endpoint at `/api/realtime` and parses incoming
  /// events as [RealtimeEvent] objects. Returns an unsubscribe function
  /// that closes the SSE connection.
  ///
  /// Parse errors on individual messages (e.g. heartbeats) are silently
  /// ignored — only valid [RealtimeEvent] JSON is dispatched to [callback].
  void Function() subscribe(
    List<String> tables,
    void Function(RealtimeEvent event) callback,
  ) {
    StreamSubscription<SseMessage>? sseSubscription;
    var cancelled = false;
    var reconnectScheduled = false;
    var reconnectAttempt = 0;
    final cancelSignal = Completer<void>();

    Uri buildRealtimeUri() {
      final queryParams = <String, String>{
        'tables': tables.join(','),
      };
      final token = client.token;
      if (token != null) {
        queryParams['token'] = token;
      }
      return Uri.parse('${client.baseUrl}/api/realtime')
          .replace(queryParameters: queryParams);
    }

    Duration computeReconnectDelay(int attempt) {
      if (_options.reconnectDelays.isEmpty) {
        return Duration.zero;
      }
      final delayIndex =
          math.min(attempt - 1, _options.reconnectDelays.length - 1);
      final baseDelay = _options.reconnectDelays[delayIndex];
      final maxJitterMs = _options.jitterMax.inMilliseconds;
      if (maxJitterMs <= 0) {
        return baseDelay;
      }

      var randomValue = _options.randomDouble?.call() ?? _random.nextDouble();
      if (randomValue < 0) randomValue = 0;
      if (randomValue > 1) randomValue = 1;
      final jitterMs = (maxJitterMs * randomValue).round();
      return baseDelay + Duration(milliseconds: jitterMs);
    }

    Future<void> sleep(Duration delay) {
      final sleepFn = _options.sleep;
      if (sleepFn != null) {
        return sleepFn(delay);
      }
      return Future<void>.delayed(delay);
    }

    late final Future<void> Function() connect;
    late final Future<void> Function() scheduleReconnect;

    scheduleReconnect = () async {
      if (cancelled || reconnectScheduled) {
        return;
      }
      if (reconnectAttempt >= _options.maxReconnectAttempts) {
        return;
      }

      reconnectScheduled = true;
      reconnectAttempt += 1;
      final delay = computeReconnectDelay(reconnectAttempt);
      try {
        await Future.any<void>(<Future<void>>[
          sleep(delay),
          cancelSignal.future,
        ]);
      } finally {
        reconnectScheduled = false;
      }

      if (cancelled) {
        return;
      }
      await connect();
    };

    connect = () async {
      if (cancelled) {
        return;
      }

      final request = http.Request('GET', buildRealtimeUri());

      late final http.StreamedResponse response;
      try {
        response = await client.httpClient.send(request);
      } catch (_) {
        await scheduleReconnect();
        return;
      }

      if (cancelled) {
        return;
      }
      if (response.statusCode < 200 || response.statusCode >= 300) {
        await scheduleReconnect();
        return;
      }

      reconnectAttempt = 0;
      final parser = SseParser(response.stream);
      sseSubscription = parser.stream.listen(
        (message) {
          if (message.data == null) return;
          try {
            final json = jsonDecode(message.data!) as JsonMap;
            callback(RealtimeEvent.fromJson(json));
          } on FormatException {
            // Ignore parse errors for heartbeat/ping messages
          } on TypeError {
            // Ignore type errors from malformed data
          }
        },
        onError: (_) {
          unawaited(scheduleReconnect());
        },
        onDone: () {
          unawaited(scheduleReconnect());
        },
      );
    };

    unawaited(connect());

    return () {
      cancelled = true;
      if (!cancelSignal.isCompleted) {
        cancelSignal.complete();
      }
      unawaited(sseSubscription?.cancel() ?? Future<void>.value());
    };
  }
}

/// TODO: Document PushClient.
class PushClient {
  PushClient(this.client);

  final AYBClient client;

  /// Register a device token for push notifications.
  Future<DeviceToken> registerDevice({
    required String appId,
    required String provider,
    required String platform,
    required String token,
    String? deviceName,
  }) {
    final body = <String, Object?>{
      'app_id': appId,
      'provider': provider,
      'platform': platform,
      'token': token,
    };
    if (deviceName != null) body['device_name'] = deviceName;

    return client.request<DeviceToken>(
      '/api/push/devices',
      method: 'POST',
      body: body,
      decode: (value) => DeviceToken.fromJson(value as JsonMap),
    );
  }

  /// List registered device tokens for an app.
  Future<List<DeviceToken>> listDevices(String appId) {
    return client.request<List<DeviceToken>>(
      '/api/push/devices?app_id=${Uri.encodeComponent(appId)}',
      decode: (value) {
        final json = value as JsonMap;
        final rawItems = json['items'];
        if (rawItems is! List<Object?>) {
          throw const FormatException(
            'Push list response is missing a valid "items" array.',
          );
        }
        return rawItems
            .map((item) => DeviceToken.fromJson(item as JsonMap))
            .toList(growable: false);
      },
    );
  }

  /// Revoke (delete) a registered device token.
  Future<void> revokeDevice(String id) {
    return client.request<void>(
      '/api/push/devices/$id',
      method: 'DELETE',
    );
  }
}

// ---------------------------------------------------------------------------
// Shared error normalization
// ---------------------------------------------------------------------------

/// Normalizes a non-2xx [http.Response] into an [AYBError].
///
/// Extracts `message`, `code`, `data`, and `doc_url`/`docUrl` from the JSON
/// body when present. Used by both [AYBClient.request] and
/// [StorageClient.upload] (which uses [http.MultipartRequest] and therefore
/// cannot share the same request path).
AYBError _normalizeHttpError(http.Response response) {
  final status = response.statusCode;
  String message = response.reasonPhrase ?? 'HTTP $status';
  String? code;
  Map<String, Object?>? data;
  String? docUrl;

  final parsed = _tryParseErrorBody(response.bodyBytes);
  if (parsed != null) {
    final parsedMessage = parsed['message'];
    if (parsedMessage is String && parsedMessage.isNotEmpty) {
      message = parsedMessage;
    }
    final parsedCode = parsed['code'];
    if (parsedCode is String && parsedCode.isNotEmpty) {
      code = parsedCode;
    } else if (parsedCode is int) {
      code = parsedCode.toString();
    }
    final parsedData = parsed['data'];
    data = _tryParseJsonMap(parsedData);
    final parsedDocUrl = parsed['doc_url'] ?? parsed['docUrl'];
    if (parsedDocUrl is String && parsedDocUrl.isNotEmpty) {
      docUrl = parsedDocUrl;
    }
  }

  return AYBError(status, message, code: code, data: data, docUrl: docUrl);
}

Map<String, Object?>? _tryParseErrorBody(List<int> bytes) {
  if (bytes.isEmpty) {
    return null;
  }
  Object? decoded;
  try {
    decoded = jsonDecode(utf8.decode(bytes));
  } on FormatException {
    return null;
  }
  return _tryParseJsonMap(decoded);
}

Map<String, Object?>? _tryParseJsonMap(Object? value) {
  if (value is! Map<Object?, Object?>) {
    return null;
  }
  final parsed = <String, Object?>{};
  for (final entry in value.entries) {
    if (entry.key is! String) {
      return null;
    }
    parsed[entry.key as String] = entry.value;
  }
  return parsed;
}
