import 'dart:async';
import 'dart:convert';

/// Event emitted when the auth store's token state changes.
class AuthStoreEvent {
  const AuthStoreEvent(this.token, this.refreshToken);

  /// The current token, or `null` if cleared.
  final String? token;

  /// The current refresh token, or `null` if cleared.
  final String? refreshToken;
}

/// In-memory auth token store with a broadcast [Stream] for state changes.
///
/// Stores an access token and refresh token. Exposes [onChange] as a
/// Dart-idiomatic broadcast stream for observing token mutations.
///
/// The [isValid] getter decodes the JWT payload and checks the `exp` claim.
///
/// For persistent storage across app restarts, use [AsyncAuthStore] which
/// adds save/clear callbacks for plugging in `flutter_secure_storage`,
/// `shared_preferences`, or any async storage backend.
class AuthStore {
  final StreamController<AuthStoreEvent> _controller =
      StreamController<AuthStoreEvent>.broadcast();

  String? _token;
  String? _refreshToken;

  /// The current access token.
  String? get token => _token;

  /// The current refresh token.
  String? get refreshToken => _refreshToken;

  /// A broadcast stream that emits [AuthStoreEvent] on every [save] or [clear].
  Stream<AuthStoreEvent> get onChange => _controller.stream;

  /// Whether the current token is a non-expired JWT.
  ///
  /// Returns `false` when no token is set, when the token is not a valid
  /// 3-part JWT, or when the `exp` claim is in the past.
  bool get isValid {
    final t = _token;
    if (t == null) return false;

    final parts = t.split('.');
    if (parts.length != 3) return false;

    try {
      final normalized = base64Url.normalize(parts[1]);
      final payloadBytes = base64Url.decode(normalized);
      final payload = jsonDecode(utf8.decode(payloadBytes));
      if (payload is! Map) return false;
      final exp = payload['exp'];
      if (exp is! int) return false;
      final expiry = DateTime.fromMillisecondsSinceEpoch(exp * 1000);
      return expiry.isAfter(DateTime.now());
    } catch (_) {
      return false;
    }
  }

  /// Store a new token pair and emit an [AuthStoreEvent].
  void save(String token, String refreshToken) {
    _token = token;
    _refreshToken = refreshToken;
    _controller.add(AuthStoreEvent(token, refreshToken));
  }

  /// Clear tokens and emit an [AuthStoreEvent] with null values.
  void clear() {
    _token = null;
    _refreshToken = null;
    _controller.add(AuthStoreEvent(null, null));
  }

  /// Close the underlying stream controller. No further events will be emitted.
  void dispose() {
    _controller.close();
  }
}

/// Auth store with async persistence callbacks for restoring sessions across
/// app restarts.
///
/// Delegates to a consumer-provided [save] callback (receives JSON-serialized
/// token data) and [clear] callback. Optionally accepts [initial] serialized
/// data to pre-load tokens from a previous session.
///
/// Example with `flutter_secure_storage`:
/// ```dart
/// final secureStorage = FlutterSecureStorage();
/// final initial = await secureStorage.read(key: 'ayb_auth');
/// final store = AsyncAuthStore(
///   save: (data) => secureStorage.write(key: 'ayb_auth', value: data),
///   clear: () => secureStorage.delete(key: 'ayb_auth'),
///   initial: initial,
/// );
/// final client = AYBClient('https://api.example.com');
/// if (store.token != null && store.refreshToken != null) {
///   client.setTokens(store.token!, store.refreshToken!);
/// }
/// ```
class AsyncAuthStore extends AuthStore {
  AsyncAuthStore({
    required Future<void> Function(String data) save,
    required Future<void> Function() clear,
    String? initial,
  })  : _saveFunc = save,
        _clearFunc = clear {
    if (initial != null) {
      _loadInitial(initial);
    }
  }

  final Future<void> Function(String data) _saveFunc;
  final Future<void> Function() _clearFunc;

  void _loadInitial(String data) {
    try {
      final decoded = jsonDecode(data);
      if (decoded is! Map) return;
      final token = decoded['token'];
      final refreshToken = decoded['refreshToken'];
      if (token is String) _token = token;
      if (refreshToken is String) _refreshToken = refreshToken;
    } on FormatException {
      // Ignore corrupt initial data.
    }
  }

  @override
  void save(String token, String refreshToken) {
    super.save(token, refreshToken);
    unawaited(
      _saveFunc(jsonEncode({'token': token, 'refreshToken': refreshToken})),
    );
  }

  @override
  void clear() {
    super.clear();
    unawaited(_clearFunc());
  }
}
