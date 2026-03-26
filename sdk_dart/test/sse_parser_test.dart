import 'dart:async';
import 'dart:convert';

import 'package:test/test.dart';

import 'package:allyourbase/src/sse/sse_parser.dart';

void main() {
  group('SseParser', () {
    test('parses single event with data', () async {
      final stream = _streamFromLines([
        'data: hello world',
        '',
      ]);
      final events = await SseParser(stream).stream.toList();
      expect(events, hasLength(1));
      expect(events[0].data, 'hello world');
      expect(events[0].event, isNull);
      expect(events[0].id, isNull);
    });

    test('parses event type', () async {
      final stream = _streamFromLines([
        'event: message',
        'data: {"foo":"bar"}',
        '',
      ]);
      final events = await SseParser(stream).stream.toList();
      expect(events, hasLength(1));
      expect(events[0].event, 'message');
      expect(events[0].data, '{"foo":"bar"}');
    });

    test('parses id field', () async {
      final stream = _streamFromLines([
        'id: 42',
        'data: test',
        '',
      ]);
      final events = await SseParser(stream).stream.toList();
      expect(events, hasLength(1));
      expect(events[0].id, '42');
      expect(events[0].data, 'test');
    });

    test('parses retry field', () async {
      final stream = _streamFromLines([
        'retry: 5000',
        'data: reconnect test',
        '',
      ]);
      final events = await SseParser(stream).stream.toList();
      expect(events, hasLength(1));
      expect(events[0].retry, 5000);
      expect(events[0].data, 'reconnect test');
    });

    test('concatenates multi-line data with newlines', () async {
      final stream = _streamFromLines([
        'data: line one',
        'data: line two',
        'data: line three',
        '',
      ]);
      final events = await SseParser(stream).stream.toList();
      expect(events, hasLength(1));
      expect(events[0].data, 'line one\nline two\nline three');
    });

    test('ignores comment lines starting with colon', () async {
      final stream = _streamFromLines([
        ': this is a comment',
        'data: real data',
        '',
      ]);
      final events = await SseParser(stream).stream.toList();
      expect(events, hasLength(1));
      expect(events[0].data, 'real data');
    });

    test('parses multiple events separated by blank lines', () async {
      final stream = _streamFromLines([
        'data: first',
        '',
        'data: second',
        '',
        'data: third',
        '',
      ]);
      final events = await SseParser(stream).stream.toList();
      expect(events, hasLength(3));
      expect(events[0].data, 'first');
      expect(events[1].data, 'second');
      expect(events[2].data, 'third');
    });

    test('handles field with no value (just field name and colon)', () async {
      final stream = _streamFromLines([
        'data:',
        '',
      ]);
      final events = await SseParser(stream).stream.toList();
      expect(events, hasLength(1));
      expect(events[0].data, '');
    });

    test('strips single leading space from field value', () async {
      final stream = _streamFromLines([
        'data:  two spaces',
        '',
      ]);
      final events = await SseParser(stream).stream.toList();
      expect(events, hasLength(1));
      // SSE spec: strip one leading space, so " two spaces" becomes " two spaces" -> only first space stripped
      expect(events[0].data, ' two spaces');
    });

    test('skips events with no data field', () async {
      final stream = _streamFromLines([
        'event: ping',
        '',
        'data: real event',
        '',
      ]);
      final events = await SseParser(stream).stream.toList();
      // First event has no data, should be skipped per SSE spec
      expect(events, hasLength(1));
      expect(events[0].data, 'real event');
    });

    test('handles stream that ends without trailing blank line', () async {
      final stream = _streamFromLines([
        'data: final event',
      ]);
      final events = await SseParser(stream).stream.toList();
      // Should flush pending event on stream close
      expect(events, hasLength(1));
      expect(events[0].data, 'final event');
    });

    test('handles empty lines between events', () async {
      final stream = _streamFromLines([
        'data: first',
        '',
        '',
        'data: second',
        '',
      ]);
      final events = await SseParser(stream).stream.toList();
      expect(events, hasLength(2));
      expect(events[0].data, 'first');
      expect(events[1].data, 'second');
    });

    test('ignores unknown fields', () async {
      final stream = _streamFromLines([
        'unknown: value',
        'data: test',
        '',
      ]);
      final events = await SseParser(stream).stream.toList();
      expect(events, hasLength(1));
      expect(events[0].data, 'test');
    });

    test('handles line with no colon (field name only)', () async {
      final stream = _streamFromLines([
        'data',
        '',
      ]);
      final events = await SseParser(stream).stream.toList();
      // Per SSE spec: line with no colon is treated as field name with empty value
      expect(events, hasLength(1));
      expect(events[0].data, '');
    });

    test('parses complex event with all fields', () async {
      final stream = _streamFromLines([
        'id: evt-123',
        'event: update',
        'retry: 3000',
        'data: {"action":"INSERT","table":"posts"}',
        '',
      ]);
      final events = await SseParser(stream).stream.toList();
      expect(events, hasLength(1));
      final evt = events[0];
      expect(evt.id, 'evt-123');
      expect(evt.event, 'update');
      expect(evt.retry, 3000);
      expect(evt.data, '{"action":"INSERT","table":"posts"}');
    });

    test('handles bytes split across chunks', () async {
      // Simulate data arriving in arbitrary byte chunks
      final fullText = 'data: chunked\n\n';
      final bytes = utf8.encode(fullText);
      // Split into small chunks
      final stream = Stream.fromIterable([
        bytes.sublist(0, 3),
        bytes.sublist(3, 8),
        bytes.sublist(8),
      ]);
      final events = await SseParser(stream).stream.toList();
      expect(events, hasLength(1));
      expect(events[0].data, 'chunked');
    });

    test('handles carriage return + line feed (CRLF)', () async {
      final text = 'data: crlf\r\n\r\n';
      final stream = Stream.value(utf8.encode(text));
      final events = await SseParser(stream).stream.toList();
      expect(events, hasLength(1));
      expect(events[0].data, 'crlf');
    });

    test('handles bare carriage return (CR)', () async {
      final text = 'data: cr\r\r';
      final stream = Stream.value(utf8.encode(text));
      final events = await SseParser(stream).stream.toList();
      expect(events, hasLength(1));
      expect(events[0].data, 'cr');
    });

    test('ignores invalid retry value', () async {
      final stream = _streamFromLines([
        'retry: not-a-number',
        'data: test',
        '',
      ]);
      final events = await SseParser(stream).stream.toList();
      expect(events, hasLength(1));
      expect(events[0].retry, isNull);
      expect(events[0].data, 'test');
    });

    test('preserves last event id across events until changed', () async {
      final stream = _streamFromLines([
        'id: evt-1',
        'data: first',
        '',
        'data: second',
        '',
      ]);
      final events = await SseParser(stream).stream.toList();

      expect(events, hasLength(2));
      expect(events[0].id, 'evt-1');
      expect(events[1].id, 'evt-1');
    });

    test('id field with empty value resets lastEventId', () async {
      final stream = _streamFromLines([
        'id: evt-1',
        'data: first',
        '',
        'id:',
        'data: second',
        '',
      ]);
      final events = await SseParser(stream).stream.toList();

      expect(events, hasLength(2));
      expect(events[0].id, 'evt-1');
      // Empty id: field resets lastEventId to '', which emits as null.
      expect(events[1].id, isNull);
    });

    test('id field containing NUL is ignored per SSE spec', () async {
      final stream = _streamFromLines([
        'id: evt-1',
        'data: first',
        '',
        'id: evil\u0000id',
        'data: second',
        '',
      ]);
      final events = await SseParser(stream).stream.toList();

      expect(events, hasLength(2));
      expect(events[0].id, 'evt-1');
      // id containing NUL is ignored; previous id persists.
      expect(events[1].id, 'evt-1');
    });

    test('id field replaced by a new value', () async {
      final stream = _streamFromLines([
        'id: evt-1',
        'data: first',
        '',
        'id: evt-2',
        'data: second',
        '',
      ]);
      final events = await SseParser(stream).stream.toList();

      expect(events, hasLength(2));
      expect(events[0].id, 'evt-1');
      expect(events[1].id, 'evt-2');
    });

    test('handles utf8 code point split across chunk boundaries', () async {
      final bytes = utf8.encode('data: cafe\u00E9\n\n');
      final splitAt = bytes.indexOf(0xC3) + 1;
      final stream = Stream.fromIterable([
        bytes.sublist(0, splitAt),
        bytes.sublist(splitAt),
      ]);

      final events = await SseParser(stream).stream.toList();
      expect(events, hasLength(1));
      expect(events[0].data, 'cafe\u00E9');
    });
  });
}

/// Helper: converts lines (without terminators) into a byte stream with LF endings.
Stream<List<int>> _streamFromLines(List<String> lines) {
  final buffer = StringBuffer();
  for (final line in lines) {
    buffer.write(line);
    buffer.write('\n');
  }
  return Stream.value(utf8.encode(buffer.toString()));
}
