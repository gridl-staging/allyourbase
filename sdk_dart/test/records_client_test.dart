import 'dart:convert';

import 'package:allyourbase/allyourbase.dart';
import 'package:test/test.dart';

import 'support/deterministic_http_client.dart';

void main() {
  late DeterministicHttpClient http;
  late AYBClient client;

  setUp(() {
    http = DeterministicHttpClient();
    client = AYBClient('https://api.example.com', httpClient: http);
    client.setTokens('test-token', 'test-refresh');
  });

  group('RecordsClient.list', () {
    test('sends GET to /api/collections/{collection}', () async {
      http.enqueue(StubResponse.json(200, {
        'items': [],
        'page': 1,
        'perPage': 20,
        'totalItems': 0,
        'totalPages': 0,
      }));

      final result = await client.records.list('posts');

      expect(http.requests, hasLength(1));
      final req = http.requests.first;
      expect(req.method, 'GET');
      expect(req.url.path, '/api/collections/posts');
      expect(req.url.query, isEmpty);
      expect(req.headers['Authorization'], 'Bearer test-token');
      expect(result.items, isEmpty);
      expect(result.page, 1);
      expect(result.perPage, 20);
      expect(result.totalItems, 0);
      expect(result.totalPages, 0);
    });

    test('parses items with decodeItem', () async {
      http.enqueue(StubResponse.json(200, {
        'items': [
          {'id': '1', 'title': 'First'},
          {'id': '2', 'title': 'Second'},
        ],
        'page': 1,
        'perPage': 20,
        'totalItems': 2,
        'totalPages': 1,
      }));

      final result = await client.records.list('posts');

      expect(result.items, hasLength(2));
      expect(result.items[0], isA<Map<String, Object?>>());
      expect((result.items[0] as Map)['id'], '1');
      expect((result.items[1] as Map)['title'], 'Second');
      expect(result.totalItems, 2);
    });

    test('sends all ListParams as query parameters', () async {
      http.enqueue(StubResponse.json(200, {
        'items': [],
        'page': 2,
        'perPage': 10,
        'totalItems': 25,
        'totalPages': 3,
      }));

      await client.records.list(
        'posts',
        params: ListParams(
          page: 2,
          perPage: 10,
          sort: '-created_at',
          filter: 'published=true',
          search: 'dart',
          fields: 'id,title',
          expand: 'author',
          skipTotal: true,
        ),
      );

      final req = http.requests.first;
      final queryParams = req.url.queryParameters;
      expect(queryParams['page'], '2');
      expect(queryParams['perPage'], '10');
      expect(queryParams['sort'], '-created_at');
      expect(queryParams['filter'], 'published=true');
      expect(queryParams['search'], 'dart');
      expect(queryParams['fields'], 'id,title');
      expect(queryParams['expand'], 'author');
      expect(queryParams['skipTotal'], 'true');
    });

    test('omits null params from query string', () async {
      http.enqueue(StubResponse.json(200, {
        'items': [],
        'page': 1,
        'perPage': 20,
        'totalItems': 0,
        'totalPages': 0,
      }));

      await client.records.list(
        'posts',
        params: ListParams(page: 3),
      );

      final req = http.requests.first;
      expect(req.url.queryParameters, {'page': '3'});
    });

    test('propagates errors', () async {
      http.enqueue(StubResponse.json(403, {
        'message': 'Forbidden',
        'code': 'auth/insufficient-permissions',
      }));

      expect(
        () => client.records.list('secret'),
        throwsA(isA<AYBError>()
            .having((e) => e.status, 'status', 403)
            .having((e) => e.message, 'message', 'Forbidden')),
      );
    });
  });

  group('RecordsClient.get', () {
    test('sends GET to /api/collections/{collection}/{id}', () async {
      http.enqueue(StubResponse.json(200, {
        'id': 'rec-1',
        'title': 'Hello',
      }));

      final result = await client.records.get('posts', 'rec-1');

      expect(http.requests, hasLength(1));
      final req = http.requests.first;
      expect(req.method, 'GET');
      expect(req.url.path, '/api/collections/posts/rec-1');
      expect(req.url.query, isEmpty);
      expect((result as Map)['id'], 'rec-1');
      expect((result as Map)['title'], 'Hello');
    });

    test('sends GetParams as query parameters', () async {
      http.enqueue(StubResponse.json(200, {'id': 'rec-1'}));

      await client.records.get(
        'posts',
        'rec-1',
        params: GetParams(fields: 'id,title', expand: 'author'),
      );

      final req = http.requests.first;
      expect(req.url.queryParameters['fields'], 'id,title');
      expect(req.url.queryParameters['expand'], 'author');
    });

    test('propagates 404 error', () async {
      http.enqueue(StubResponse.json(404, {
        'message': 'Record not found',
        'code': 'not_found',
      }));

      expect(
        () => client.records.get('posts', 'missing'),
        throwsA(isA<AYBError>()
            .having((e) => e.status, 'status', 404)
            .having((e) => e.message, 'message', 'Record not found')),
      );
    });
  });

  group('RecordsClient.create', () {
    test('sends POST to /api/collections/{collection} with JSON body', () async {
      http.enqueue(StubResponse.json(201, {
        'id': 'new-1',
        'title': 'New Post',
        'published': false,
      }));

      final result = await client.records.create('posts', {
        'title': 'New Post',
        'published': false,
      });

      expect(http.requests, hasLength(1));
      final req = http.requests.first;
      expect(req.method, 'POST');
      expect(req.url.path, '/api/collections/posts');
      expect(req.headers['content-type'], 'application/json');
      expect(req.headers['Authorization'], 'Bearer test-token');

      final body = jsonDecode(req.body) as Map<String, Object?>;
      expect(body['title'], 'New Post');
      expect(body['published'], false);

      expect((result as Map)['id'], 'new-1');
    });

    test('propagates validation errors', () async {
      http.enqueue(StubResponse.json(400, {
        'message': 'Validation failed',
        'code': 'validation_error',
        'data': {'title': 'required'},
      }));

      expect(
        () => client.records.create('posts', {}),
        throwsA(isA<AYBError>()
            .having((e) => e.status, 'status', 400)
            .having((e) => e.code, 'code', 'validation_error')
            .having((e) => e.data, 'data', {'title': 'required'})),
      );
    });
  });

  group('RecordsClient.update', () {
    test('sends PATCH to /api/collections/{collection}/{id} with JSON body', () async {
      http.enqueue(StubResponse.json(200, {
        'id': 'rec-1',
        'title': 'Updated Title',
        'published': true,
      }));

      final result = await client.records.update('posts', 'rec-1', {
        'title': 'Updated Title',
        'published': true,
      });

      expect(http.requests, hasLength(1));
      final req = http.requests.first;
      expect(req.method, 'PATCH');
      expect(req.url.path, '/api/collections/posts/rec-1');
      expect(req.headers['content-type'], 'application/json');

      final body = jsonDecode(req.body) as Map<String, Object?>;
      expect(body['title'], 'Updated Title');
      expect(body['published'], true);

      expect((result as Map)['title'], 'Updated Title');
    });

    test('propagates 404 error for missing record', () async {
      http.enqueue(StubResponse.json(404, {
        'message': 'Record not found',
      }));

      expect(
        () => client.records.update('posts', 'gone', {'title': 'x'}),
        throwsA(isA<AYBError>().having((e) => e.status, 'status', 404)),
      );
    });
  });

  group('RecordsClient.delete', () {
    test('sends DELETE to /api/collections/{collection}/{id}', () async {
      http.enqueue(StubResponse.empty(204));

      await client.records.delete('posts', 'rec-1');

      expect(http.requests, hasLength(1));
      final req = http.requests.first;
      expect(req.method, 'DELETE');
      expect(req.url.path, '/api/collections/posts/rec-1');
      expect(req.headers['Authorization'], 'Bearer test-token');
    });

    test('propagates 404 error', () async {
      http.enqueue(StubResponse.json(404, {
        'message': 'Record not found',
      }));

      expect(
        () => client.records.delete('posts', 'missing'),
        throwsA(isA<AYBError>().having((e) => e.status, 'status', 404)),
      );
    });
  });

  group('RecordsClient.batch', () {
    test('sends POST to /api/collections/{collection}/batch with operations', () async {
      http.enqueue(StubResponse.json(200, [
        {'index': 0, 'status': 201, 'body': {'id': 'new-1', 'title': 'A'}},
        {'index': 1, 'status': 200, 'body': {'id': 'rec-2', 'title': 'B Updated'}},
        {'index': 2, 'status': 204, 'body': null},
      ]));

      final operations = [
        BatchOperation(method: 'create', body: {'title': 'A'}),
        BatchOperation(method: 'update', id: 'rec-2', body: {'title': 'B Updated'}),
        BatchOperation(method: 'delete', id: 'rec-3'),
      ];

      final results = await client.records.batch('posts', operations);

      expect(http.requests, hasLength(1));
      final req = http.requests.first;
      expect(req.method, 'POST');
      expect(req.url.path, '/api/collections/posts/batch');
      expect(req.headers['content-type'], 'application/json');

      final body = jsonDecode(req.body) as Map<String, Object?>;
      final ops = body['operations'] as List;
      expect(ops, hasLength(3));
      expect((ops[0] as Map)['method'], 'create');
      expect((ops[0] as Map).containsKey('id'), isFalse,
          reason: 'create operation should omit null id');
      expect((ops[1] as Map)['id'], 'rec-2');
      expect((ops[2] as Map)['method'], 'delete');
      expect((ops[2] as Map).containsKey('body'), isFalse,
          reason: 'delete operation should omit null body');

      expect(results, hasLength(3));
      expect(results[0].index, 0);
      expect(results[0].status, 201);
      expect(results[0].body, isNotNull);
      expect((results[0].body as Map)['id'], 'new-1');
      expect(results[1].index, 1);
      expect(results[1].status, 200);
      expect(results[2].index, 2);
      expect(results[2].status, 204);
      expect(results[2].body, isNull);
    });

    test('propagates batch-level errors', () async {
      http.enqueue(StubResponse.json(400, {
        'message': 'Too many operations',
        'code': 'batch/limit_exceeded',
      }));

      expect(
        () => client.records.batch('posts', []),
        throwsA(isA<AYBError>()
            .having((e) => e.status, 'status', 400)
            .having((e) => e.code, 'code', 'batch/limit_exceeded')),
      );
    });
  });

  group('ListParams', () {
    test('builds query map with all fields', () {
      final params = ListParams(
        page: 1,
        perPage: 25,
        sort: '-created_at',
        filter: 'active=true',
        search: 'hello',
        fields: 'id,name',
        expand: 'author',
        skipTotal: true,
      );

      final map = params.toQueryMap();
      expect(map, {
        'page': '1',
        'perPage': '25',
        'sort': '-created_at',
        'filter': 'active=true',
        'search': 'hello',
        'fields': 'id,name',
        'expand': 'author',
        'skipTotal': 'true',
      });
    });

    test('omits null fields', () {
      final params = ListParams(page: 2, sort: 'name');
      final map = params.toQueryMap();
      expect(map, {'page': '2', 'sort': 'name'});
    });

    test('does not include skipTotal when false', () {
      final params = ListParams(skipTotal: false);
      expect(params.toQueryMap(), isEmpty);
    });

    test('empty params produce empty map', () {
      final params = ListParams();
      expect(params.toQueryMap(), isEmpty);
    });
  });

  group('GetParams', () {
    test('builds query map with both fields', () {
      final params = GetParams(fields: 'id,title', expand: 'tags');
      expect(params.toQueryMap(), {
        'fields': 'id,title',
        'expand': 'tags',
      });
    });

    test('omits null fields', () {
      final params = GetParams(expand: 'author');
      expect(params.toQueryMap(), {'expand': 'author'});
    });

    test('empty params produce empty map', () {
      final params = GetParams();
      expect(params.toQueryMap(), isEmpty);
    });
  });

  group('RecordsClient with no auth token', () {
    test('does not send Authorization header when no tokens set', () async {
      final noAuthClient = AYBClient('https://api.example.com', httpClient: http);
      http.enqueue(StubResponse.json(200, {
        'items': [],
        'page': 1,
        'perPage': 20,
        'totalItems': 0,
        'totalPages': 0,
      }));

      await noAuthClient.records.list('public_data');

      final req = http.requests.first;
      expect(req.headers.containsKey('Authorization'), isFalse);
    });
  });

  group('RecordsClient with API key auth', () {
    test('sends API key as Bearer token', () async {
      final apiKeyClient = AYBClient('https://api.example.com', httpClient: http);
      apiKeyClient.setApiKey('ayb_abc123');
      http.enqueue(StubResponse.json(200, {'id': 'rec-1'}));

      await apiKeyClient.records.get('posts', 'rec-1');

      final req = http.requests.first;
      expect(req.headers['Authorization'], 'Bearer ayb_abc123');
    });
  });
}
