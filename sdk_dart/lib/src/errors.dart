/// TODO: Document AYBError.
class AYBError implements Exception {
  const AYBError(
    this.status,
    this.message, {
    this.code,
    this.data,
    this.docUrl,
  });

  final int status;
  final String message;
  final String? code;
  final Map<String, Object?>? data;
  final String? docUrl;

  @override
  String toString() {
    final codePart = code == null ? '' : ', code=$code';
    return 'AYBError(status=$status, message=$message$codePart)';
  }
}
