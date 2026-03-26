import 'dart:async';
import 'dart:convert';

import 'package:http/http.dart' as http;

/// HTTP client that returns streaming responses for SSE testing.
///
/// Unlike [DeterministicHttpClient], this returns [StreamedResponse] backed
/// by a [StreamController] so tests can push SSE data incrementally.
class StreamingHttpClient extends http.BaseClient {
  final List<RecordedStreamRequest> requests = <RecordedStreamRequest>[];
  final List<_PreparedStream> _prepared = <_PreparedStream>[];
  final List<_RequestWaiter> _requestWaiters = <_RequestWaiter>[];

  /// Prepare a streaming response that will be returned for the next request.
  /// Returns a controller for pushing SSE data.
  SseStreamController prepareStream(int statusCode) {
    final byteController = StreamController<List<int>>();
    final sseController = SseStreamController(byteController);
    _prepared.add(_PreparedStream(statusCode, byteController.stream));
    return sseController;
  }

  /// Wait for a request to be received.
  Future<void> waitForRequest() {
    return waitForRequestCount(1);
  }

  /// Wait until at least [count] requests have been received.
  Future<void> waitForRequestCount(int count) {
    if (requests.length >= count) {
      return Future<void>.value();
    }
    final completer = Completer<void>();
    _requestWaiters.add(_RequestWaiter(count, completer));
    return completer.future;
  }

  @override
  Future<http.StreamedResponse> send(http.BaseRequest request) async {
    if (_prepared.isEmpty) {
      throw StateError(
        'No prepared stream for ${request.method} ${request.url}',
      );
    }

    requests.add(RecordedStreamRequest(
      method: request.method,
      url: request.url,
      headers: Map<String, String>.from(request.headers),
    ));

    // Notify waiters.
    final remaining = <_RequestWaiter>[];
    for (final waiter in _requestWaiters) {
      if (requests.length >= waiter.count) {
        waiter.completer.complete();
      } else {
        remaining.add(waiter);
      }
    }
    _requestWaiters
      ..clear()
      ..addAll(remaining);

    final prepared = _prepared.removeAt(0);
    return http.StreamedResponse(
      prepared.stream,
      prepared.statusCode,
      request: request,
    );
  }
}

class _RequestWaiter {
  const _RequestWaiter(this.count, this.completer);

  final int count;
  final Completer<void> completer;
}

class RecordedStreamRequest {
  const RecordedStreamRequest({
    required this.method,
    required this.url,
    required this.headers,
  });

  final String method;
  final Uri url;
  final Map<String, String> headers;
}

/// Controller for pushing SSE-formatted data into a streaming response.
class SseStreamController {
  SseStreamController(this._byteController);

  final StreamController<List<int>> _byteController;
  bool _closed = false;

  bool get isClosed => _closed;

  /// Push a complete SSE event into the stream.
  void addSseEvent({String? event, required String data, String? id}) {
    final buffer = StringBuffer();
    if (event != null) {
      buffer.writeln('event: $event');
    }
    if (id != null) {
      buffer.writeln('id: $id');
    }
    for (final line in data.split('\n')) {
      buffer.writeln('data: $line');
    }
    buffer.writeln(); // blank line to end event
    _byteController.add(utf8.encode(buffer.toString()));
  }

  /// Push raw text into the stream.
  void addRaw(String text) {
    _byteController.add(utf8.encode(text));
  }

  void close() {
    _closed = true;
    _byteController.close();
  }
}

class _PreparedStream {
  const _PreparedStream(this.statusCode, this.stream);

  final int statusCode;
  final Stream<List<int>> stream;
}
