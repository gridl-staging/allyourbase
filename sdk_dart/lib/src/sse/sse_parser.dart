import 'dart:async';
import 'dart:convert';

/// A parsed Server-Sent Event.
class SseMessage {
  const SseMessage({
    this.event,
    this.data,
    this.id,
    this.retry,
  });

  /// The event type (from the `event:` field), or null for default messages.
  final String? event;

  /// The event data (from `data:` field(s), joined by newlines).
  final String? data;

  /// The last event ID (from the `id:` field).
  final String? id;

  /// Reconnection time in milliseconds (from the `retry:` field).
  final int? retry;
}

/// Parses a byte stream (e.g. from an HTTP streaming response) into
/// Server-Sent Events per the W3C EventSource specification.
///
/// Handles:
/// - LF, CR, and CRLF line endings
/// - Multi-line `data:` fields (joined with newlines)
/// - Comment lines (`:` prefix) — silently ignored
/// - `event:`, `id:`, `retry:` fields
/// - Single leading space stripping from field values
/// - Events with no `data:` field are not emitted
/// - Bytes split across arbitrary chunk boundaries
class SseParser {
  SseParser(Stream<List<int>> byteStream) {
    _subscription = byteStream.cast<List<int>>().transform(utf8.decoder).listen(
      _onData,
      onDone: _onDone,
      onError: _controller.addError,
      cancelOnError: false,
    );
    _controller.onCancel = _cancel;
  }

  final StreamController<SseMessage> _controller =
      StreamController<SseMessage>();
  late final StreamSubscription<String> _subscription;

  // Buffered line state
  final StringBuffer _lineBuffer = StringBuffer();
  bool _prevCr = false;

  // Current event being built
  String? _eventType;
  List<String>? _dataLines;
  String _lastId = '';
  int? _retry;

  /// The stream of parsed SSE messages.
  Stream<SseMessage> get stream => _controller.stream;

  void _onData(String text) {
    for (var i = 0; i < text.length; i++) {
      final char = text[i];

      if (_prevCr) {
        _prevCr = false;
        // If CR was followed by LF, skip the LF (already processed the line)
        if (char == '\n') continue;
      }

      if (char == '\r') {
        _prevCr = true;
        _processLine(_lineBuffer.toString());
        _lineBuffer.clear();
      } else if (char == '\n') {
        _processLine(_lineBuffer.toString());
        _lineBuffer.clear();
      } else {
        _lineBuffer.write(char);
      }
    }
  }

  void _onDone() {
    // Flush any remaining buffered line
    if (_lineBuffer.isNotEmpty) {
      _processLine(_lineBuffer.toString());
      _lineBuffer.clear();
    }
    // Flush any pending event
    _dispatchEvent();
    _controller.close();
  }

  Future<void> _cancel() async {
    await _subscription.cancel();
  }

  void _processLine(String line) {
    // Blank line = dispatch event
    if (line.isEmpty) {
      _dispatchEvent();
      return;
    }

    // Comment line
    if (line.startsWith(':')) {
      return;
    }

    // Parse field:value
    String field;
    String value;
    final colonIndex = line.indexOf(':');
    if (colonIndex == -1) {
      // No colon: field name is entire line, value is empty string
      field = line;
      value = '';
    } else {
      field = line.substring(0, colonIndex);
      value = line.substring(colonIndex + 1);
      // Strip single leading space from value
      if (value.startsWith(' ')) {
        value = value.substring(1);
      }
    }

    switch (field) {
      case 'event':
        _eventType = value;
        break;
      case 'data':
        _dataLines ??= <String>[];
        _dataLines!.add(value);
        break;
      case 'id':
        // Ignore values containing NUL per SSE spec.
        if (!value.contains('\u0000')) {
          _lastId = value;
        }
        break;
      case 'retry':
        final parsed = int.tryParse(value);
        if (parsed != null && parsed >= 0) {
          _retry = parsed;
        }
        break;
      // Unknown fields are ignored per spec
    }
  }

  void _dispatchEvent() {
    // Per SSE spec: if data buffer is null (no data field), do not dispatch
    if (_dataLines == null) {
      _eventType = null;
      _retry = null;
      return;
    }

    final data = _dataLines!.join('\n');
    _controller.add(SseMessage(
      event: _eventType,
      data: data,
      id: _lastId.isEmpty ? null : _lastId,
      retry: _retry,
    ));

    // Reset per-event state
    _eventType = null;
    _dataLines = null;
    _retry = null;
  }
}
