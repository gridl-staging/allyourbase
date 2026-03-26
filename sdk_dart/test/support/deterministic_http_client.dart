import 'dart:convert';

import 'package:http/http.dart' as http;

/// TODO: Document DeterministicHttpClient.
class DeterministicHttpClient extends http.BaseClient {
  DeterministicHttpClient([Iterable<StubResponse> initialResponses = const []]) {
    _responses.addAll(initialResponses);
  }

  final List<StubResponse> _responses = <StubResponse>[];
  final List<RecordedRequest> requests = <RecordedRequest>[];

  void enqueue(StubResponse response) {
    _responses.add(response);
  }

  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) async {
    if (_responses.isEmpty) {
      throw StateError(
        'No stubbed response queued for ${request.method} ${request.url}',
      );
    }

    final requestBodyBytes = await request.finalize().toBytes();
    requests.add(
      RecordedRequest(
        method: request.method,
        url: request.url,
        headers: Map<String, String>.from(request.headers),
        bodyBytes: requestBodyBytes,
      ),
    );

    final response = _responses.removeAt(0);
    return http.StreamedResponse(
      Stream<List<int>>.value(response.bodyBytes),
      response.statusCode,
      headers: response.headers,
      reasonPhrase: response.reasonPhrase,
      request: request,
    );
  }
}

/// TODO: Document RecordedRequest.
class RecordedRequest {
  const RecordedRequest({
    required this.method,
    required this.url,
    required this.headers,
    required this.bodyBytes,
  });

  final String method;
  final Uri url;
  final Map<String, String> headers;
  final List<int> bodyBytes;

  String get body => utf8.decode(bodyBytes, allowMalformed: true);

  Object? decodeJsonBody() {
    if (bodyBytes.isEmpty) {
      return null;
    }
    return jsonDecode(body);
  }
}

/// TODO: Document StubResponse.
class StubResponse {
  const StubResponse({
    required this.statusCode,
    required this.bodyBytes,
    this.headers = const <String, String>{},
    this.reasonPhrase,
  });

  factory StubResponse.empty(
    int statusCode, {
    Map<String, String>? headers,
    String? reasonPhrase,
  }) {
    return StubResponse(
      statusCode: statusCode,
      bodyBytes: const <int>[],
      headers: headers ?? const <String, String>{},
      reasonPhrase: reasonPhrase,
    );
  }

  factory StubResponse.text(
    int statusCode,
    String body, {
    Map<String, String>? headers,
    String? reasonPhrase,
  }) {
    return StubResponse(
      statusCode: statusCode,
      bodyBytes: utf8.encode(body),
      headers: headers ?? const <String, String>{},
      reasonPhrase: reasonPhrase,
    );
  }

  factory StubResponse.json(
    int statusCode,
    Object? body, {
    Map<String, String>? headers,
    String? reasonPhrase,
  }) {
    final mergedHeaders = <String, String>{
      'content-type': 'application/json',
      ...?headers,
    };
    return StubResponse(
      statusCode: statusCode,
      bodyBytes: utf8.encode(jsonEncode(body)),
      headers: mergedHeaders,
      reasonPhrase: reasonPhrase,
    );
  }

  final int statusCode;
  final List<int> bodyBytes;
  final Map<String, String> headers;
  final String? reasonPhrase;
}
