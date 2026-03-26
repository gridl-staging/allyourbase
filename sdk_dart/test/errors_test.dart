import 'package:allyourbase/allyourbase.dart';
import 'package:test/test.dart';

void main() {
  group('AYBError', () {
    test('exposes status, code, data, and docUrl fields', () {
      final error = AYBError(
        401,
        'Unauthorized',
        code: 'auth/unauthorized',
        data: const {'field': 'token'},
        docUrl: 'https://allyourbase.io/docs/errors#auth-unauthorized',
      );

      expect(error.status, 401);
      expect(error.message, 'Unauthorized');
      expect(error.code, 'auth/unauthorized');
      expect(error.data, const {'field': 'token'});
      expect(
        error.docUrl,
        'https://allyourbase.io/docs/errors#auth-unauthorized',
      );
    });

    test('toString formats status, message, and optional code', () {
      final error = AYBError(500, 'Something broke');
      expect(error.toString(), 'AYBError(status=500, message=Something broke)');

      final errorWithCode = AYBError(401, 'Unauthorized', code: 'auth/bad');
      expect(
        errorWithCode.toString(),
        'AYBError(status=401, message=Unauthorized, code=auth/bad)',
      );
    });
  });
}
