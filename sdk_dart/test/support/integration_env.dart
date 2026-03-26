import 'dart:io';

class IntegrationCredentials {
  const IntegrationCredentials({
    required this.email,
    required this.password,
  });

  final String email;
  final String password;
}

/// TODO: Document IntegrationTestEnv.
class IntegrationTestEnv {
  IntegrationTestEnv._({
    required this.baseUrl,
    required this.timeout,
  });

  static const Duration defaultTimeout = Duration(seconds: 30);

  final String? baseUrl;
  final Duration timeout;
  int _credentialCounter = 0;

  bool get isConfigured => baseUrl != null;

  String? get skipReason {
    if (isConfigured) {
      return null;
    }
    return 'Set AYB_TEST_URL to run integration tests.';
  }

  static IntegrationTestEnv fromEnvironment([Map<String, String>? environment]) {
    final env = environment ?? Platform.environment;
    final rawBaseUrl = env['AYB_TEST_URL']?.trim();
    final normalizedBaseUrl = _normalizeUrl(rawBaseUrl);
    final timeout = _parseTimeout(env['AYB_TEST_TIMEOUT_SECONDS']);
    return IntegrationTestEnv._(
      baseUrl: normalizedBaseUrl,
      timeout: timeout,
    );
  }

  IntegrationCredentials newCredentials(String scenario) {
    _credentialCounter += 1;
    final scenarioSlug = _slugifyScenario(scenario);
    final timestamp = DateTime.now().microsecondsSinceEpoch.toRadixString(36);
    final suffix = '$timestamp-${_credentialCounter.toRadixString(36)}';
    return IntegrationCredentials(
      email: 'sdk-dart-$scenarioSlug-$suffix@example.test',
      password: 'P@ssw0rd-$suffix',
    );
  }

  static String? _normalizeUrl(String? rawBaseUrl) {
    if (rawBaseUrl == null || rawBaseUrl.isEmpty) {
      return null;
    }

    var normalized = rawBaseUrl;
    while (normalized.endsWith('/')) {
      normalized = normalized.substring(0, normalized.length - 1);
    }
    return normalized.isEmpty ? null : normalized;
  }

  static Duration _parseTimeout(String? rawTimeoutSeconds) {
    final seconds = int.tryParse(rawTimeoutSeconds ?? '');
    if (seconds == null || seconds <= 0) {
      return defaultTimeout;
    }
    return Duration(seconds: seconds);
  }

  static String _slugifyScenario(String scenario) {
    final buffer = StringBuffer();
    var previousWasHyphen = false;

    for (final rune in scenario.runes) {
      final char = String.fromCharCode(rune).toLowerCase();
      final isAlphaNum = (char.codeUnitAt(0) >= 48 && char.codeUnitAt(0) <= 57) ||
          (char.codeUnitAt(0) >= 97 && char.codeUnitAt(0) <= 122);

      if (isAlphaNum) {
        buffer.write(char);
        previousWasHyphen = false;
        continue;
      }

      if (!previousWasHyphen) {
        buffer.write('-');
        previousWasHyphen = true;
      }
    }

    final slug = buffer.toString().replaceAll(RegExp(r'^-+|-+$'), '');
    return slug.isEmpty ? 'scenario' : slug;
  }
}
