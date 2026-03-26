import Foundation

public struct SseMessage {
    public let event: String?
    public let data: String?
    public let id: String?
    public let retry: Int?

    public init(event: String? = nil, data: String? = nil, id: String? = nil, retry: Int? = nil) {
        self.event = event
        self.data = data
        self.id = id
        self.retry = retry
    }
}

public struct SseParser<Bytes: AsyncSequence> where Bytes.Element == UInt8, Bytes: Sendable {
    private let bytes: Bytes

    public init(bytes: Bytes) {
        self.bytes = bytes
    }

    public func messages() -> AsyncThrowingStream<SseMessage, Error> {
        let inputBytes = bytes
        return AsyncThrowingStream<SseMessage, Error> { continuation in
            Task {
                do {
                    var buffer: [UInt8] = []
                    var currentEvent: String?
                    var currentData: String?
                    var currentId: String?
                    var currentRetry: Int?
                    var hasField = false

                    func flush() {
                        guard hasField else {
                            return
                        }
                        continuation.yield(
                            SseMessage(
                                event: currentEvent,
                                data: currentData,
                                id: currentId,
                                retry: currentRetry
                            )
                        )
                        currentEvent = nil
                        currentData = nil
                        currentId = nil
                        currentRetry = nil
                        hasField = false
                    }

                    func process(line: String) {
                        if line.isEmpty {
                            flush()
                            return
                        }
                        if line.hasPrefix(":") {
                            return
                        }
                        guard let separator = line.firstIndex(of: ":") else {
                            return
                        }

                        let field = String(line[..<separator])
                        var value = String(line[line.index(after: separator)...])
                        if value.hasPrefix(" ") {
                            value.removeFirst()
                        }

                        switch field {
                        case "event":
                            currentEvent = value
                            hasField = true
                        case "data":
                            if let existing = currentData {
                                currentData = "\(existing)\n\(value)"
                            } else {
                                currentData = value
                            }
                            hasField = true
                        case "id":
                            currentId = value
                            hasField = true
                        case "retry":
                            if let retry = Int(value) {
                                currentRetry = retry
                                hasField = true
                            }
                        default:
                            break
                        }
                    }

                    for try await byte in inputBytes {
                        if byte == 10 { // '\n'
                            process(line: String(decoding: buffer, as: UTF8.self))
                            buffer.removeAll(keepingCapacity: true)
                        } else if byte != 13 { // ignore '\r'
                            buffer.append(byte)
                        }
                    }

                    if !buffer.isEmpty {
                        process(line: String(decoding: buffer, as: UTF8.self))
                    }
                    flush()
                    continuation.finish()
                } catch is CancellationError {
                    continuation.finish()
                } catch {
                    continuation.finish(throwing: error)
                }
            }
        }
    }
}

public final class RealtimeClient: @unchecked Sendable {
    private unowned let client: AYBClient
    private let sseTransport: SSETransport
    private let wsTransport: WebSocketTransport
    private let options: RealtimeOptions
    private let jitterProvider: @Sendable (TimeInterval) -> TimeInterval
    private let sleep: @Sendable (UInt64) async throws -> Void
    private let wsPingInterval: TimeInterval

    private final class CallbackBox: @unchecked Sendable {
        let callback: (RealtimeEvent) -> Void

        init(callback: @escaping (RealtimeEvent) -> Void) {
            self.callback = callback
        }
    }

    private final class SubscriptionState: @unchecked Sendable {
        private let lock = NSLock()
        private var task: Task<Void, Never>?
        private var connection: (any SSEConnection)?

        func setTask(_ task: Task<Void, Never>) {
            lock.lock()
            self.task = task
            lock.unlock()
        }

        func setConnection(_ connection: any SSEConnection) {
            lock.lock()
            self.connection = connection
            lock.unlock()
        }

        func clearConnection() {
            lock.lock()
            self.connection = nil
            lock.unlock()
        }

        func cancel() {
            lock.lock()
            let task = self.task
            let connection = self.connection
            lock.unlock()
            task?.cancel()
            connection?.cancel()
        }
    }

    private final class WebSocketState: @unchecked Sendable {
        struct PresenceSyncResult: @unchecked Sendable {
            let value: [String: Any]
        }

        private let lock = NSLock()
        private var connection: (any WebSocketConnection)?
        private var receiveTask: Task<Void, Never>?
        private var pingTask: Task<Void, Never>?
        private var isConnected = false
        private var refCounter = 0
        private var pendingReplies: [String: CheckedContinuation<WebSocketServerMessage, Error>] = [:]
        private var bufferedReplies: [String: WebSocketServerMessage] = [:]
        private var pendingPresenceSync: [String: CheckedContinuation<PresenceSyncResult, Error>] = [:]
        private var lastPresenceSync: [String: [String: [String: Any]]] = [:]
        private var tableCallbacks: [UUID: (tables: Set<String>, callback: (RealtimeEvent) -> Void)] = [:]
        private var broadcastCallbacks: [String: [UUID: (String, [String: Any]) -> Void]] = [:]
        private var channelSubscriptions: Set<String> = []

        private func withLock<T>(_ body: () -> T) -> T {
            lock.lock()
            defer { lock.unlock() }
            return body()
        }

        private func makeAnyPresenceMap(_ presences: [String: [String: Any]]) -> [String: Any] {
            var anyMap: [String: Any] = [:]
            for (connID, payload) in presences {
                anyMap[connID] = payload
            }
            return anyMap
        }

        func nextRef() -> String {
            withLock {
                refCounter += 1
                return "r\(refCounter)"
            }
        }

        func setConnection(_ connection: any WebSocketConnection, receiveTask: Task<Void, Never>, pingTask: Task<Void, Never>) {
            withLock {
                self.connection = connection
                self.receiveTask = receiveTask
                self.pingTask = pingTask
                self.isConnected = true
            }
        }

        func connected() -> Bool {
            withLock { isConnected }
        }

        func connectionSnapshot() -> (any WebSocketConnection)? {
            withLock { connection }
        }

        func markDisconnected(_ error: Error? = nil) {
            let snapshots: (
                replies: [CheckedContinuation<WebSocketServerMessage, Error>],
                syncs: [CheckedContinuation<PresenceSyncResult, Error>],
                receiveTask: Task<Void, Never>?,
                pingTask: Task<Void, Never>?
            ) = withLock {
                let receive = receiveTask
                let ping = pingTask
                isConnected = false
                connection = nil
                receiveTask = nil
                pingTask = nil
                channelSubscriptions.removeAll()
                let replies = Array(pendingReplies.values)
                let syncs = Array(pendingPresenceSync.values)
                pendingReplies.removeAll()
                bufferedReplies.removeAll()
                pendingPresenceSync.removeAll()
                lastPresenceSync.removeAll()
                return (replies, syncs, receive, ping)
            }
            snapshots.receiveTask?.cancel()
            snapshots.pingTask?.cancel()
            guard let error else {
                return
            }
            for continuation in snapshots.replies {
                continuation.resume(throwing: error)
            }
            for continuation in snapshots.syncs {
                continuation.resume(throwing: error)
            }
        }

        func storeReplyContinuation(_ continuation: CheckedContinuation<WebSocketServerMessage, Error>, ref: String) {
            let buffered: WebSocketServerMessage? = withLock {
                if let buffered = bufferedReplies.removeValue(forKey: ref) {
                    return buffered
                }
                pendingReplies[ref] = continuation
                return nil
            }
            if let buffered {
                continuation.resume(returning: buffered)
            }
        }

        func failReplyContinuation(ref: String, error: Error) {
            let continuation: CheckedContinuation<WebSocketServerMessage, Error>? = withLock {
                pendingReplies.removeValue(forKey: ref)
            }
            continuation?.resume(throwing: error)
        }

        func resolveReply(ref: String, message: WebSocketServerMessage) {
            let continuation: CheckedContinuation<WebSocketServerMessage, Error>? = withLock {
                let pending = pendingReplies.removeValue(forKey: ref)
                if pending == nil {
                    bufferedReplies[ref] = message
                }
                return pending
            }
            continuation?.resume(returning: message)
        }

        func storePresenceSyncContinuation(_ continuation: CheckedContinuation<PresenceSyncResult, Error>, channel: String) {
            let cached: [String: [String: Any]]? = withLock {
                if let cached = lastPresenceSync.removeValue(forKey: channel) {
                    return cached
                }
                pendingPresenceSync[channel] = continuation
                return nil
            }
            guard let cached else {
                return
            }
            continuation.resume(returning: PresenceSyncResult(value: makeAnyPresenceMap(cached)))
        }

        func cancelPresenceSync(channel: String, error: Error) {
            let continuation: CheckedContinuation<PresenceSyncResult, Error>? = withLock {
                pendingPresenceSync.removeValue(forKey: channel)
            }
            continuation?.resume(throwing: error)
        }

        func resolvePresenceSync(channel: String, presences: [String: [String: Any]]) {
            let continuation: CheckedContinuation<PresenceSyncResult, Error>? = withLock {
                let pending = pendingPresenceSync.removeValue(forKey: channel)
                if pending == nil {
                    lastPresenceSync[channel] = presences
                }
                return pending
            }
            guard let continuation else {
                return
            }
            continuation.resume(returning: PresenceSyncResult(value: makeAnyPresenceMap(presences)))
        }

        func addTableCallback(id: UUID, tables: Set<String>, callback: @escaping (RealtimeEvent) -> Void) {
            withLock {
                tableCallbacks[id] = (tables: tables, callback: callback)
            }
        }

        func removeTableCallback(id: UUID) {
            _ = withLock {
                tableCallbacks.removeValue(forKey: id)
            }
        }

        func dispatchTableEvent(_ event: RealtimeEvent) {
            let callbacks: [(RealtimeEvent) -> Void] = withLock {
                tableCallbacks.values
                    .filter { $0.tables.contains(event.table) }
                    .map { $0.callback }
            }
            for callback in callbacks {
                callback(event)
            }
        }

        func addChannelSubscription(_ channel: String) {
            _ = withLock {
                channelSubscriptions.insert(channel)
            }
        }

        func removeChannelSubscription(_ channel: String) {
            _ = withLock {
                channelSubscriptions.remove(channel)
            }
        }

        func isChannelSubscribed(_ channel: String) -> Bool {
            withLock {
                channelSubscriptions.contains(channel)
            }
        }

        func addBroadcastCallback(channel: String, id: UUID, callback: @escaping (String, [String: Any]) -> Void) {
            withLock {
                var callbacks = broadcastCallbacks[channel] ?? [:]
                callbacks[id] = callback
                broadcastCallbacks[channel] = callbacks
            }
        }

        func removeBroadcastCallback(channel: String, id: UUID) {
            withLock {
                var callbacks = broadcastCallbacks[channel] ?? [:]
                callbacks.removeValue(forKey: id)
                if callbacks.isEmpty {
                    broadcastCallbacks.removeValue(forKey: channel)
                } else {
                    broadcastCallbacks[channel] = callbacks
                }
            }
        }

        func dispatchBroadcast(channel: String, event: String, payload: [String: Any]) {
            let callbacks: [(String, [String: Any]) -> Void] = withLock {
                Array((broadcastCallbacks[channel] ?? [:]).values)
            }
            for callback in callbacks {
                callback(event, payload)
            }
        }
    }

    private let wsState = WebSocketState()

    init(
        client: AYBClient,
        sseTransport: SSETransport? = nil,
        wsTransport: WebSocketTransport? = nil,
        options: RealtimeOptions = RealtimeOptions(),
        jitterProvider: @escaping @Sendable (TimeInterval) -> TimeInterval = { max in
            guard max > 0 else { return 0 }
            return Double.random(in: 0...max)
        },
        sleep: @escaping @Sendable (UInt64) async throws -> Void = { nanos in
            try await Task.sleep(nanoseconds: nanos)
        },
        wsPingInterval: TimeInterval = 25
    ) {
        self.client = client
        self.sseTransport = sseTransport ?? client.sseTransport
        self.wsTransport = wsTransport ?? client.wsTransport
        self.options = options
        self.jitterProvider = jitterProvider
        self.sleep = sleep
        self.wsPingInterval = wsPingInterval
    }

    // MARK: SSE

    public func subscribe(
        tables: [String],
        filter: String? = nil,
        callback: @escaping (RealtimeEvent) -> Void
    ) -> () -> Void {
        guard !tables.isEmpty else {
            return {}
        }

        let subscription = SubscriptionState()
        let callbackBox = CallbackBox(callback: callback)
        let task = Task {
            await runSSELoop(
                tables: tables,
                filter: filter,
                callbackBox: callbackBox,
                subscription: subscription
            )
        }
        subscription.setTask(task)

        return {
            subscription.cancel()
        }
    }

    private func runSSELoop(
        tables: [String],
        filter: String?,
        callbackBox: CallbackBox,
        subscription: SubscriptionState
    ) async {
        var reconnectAttempt = 0

        while !Task.isCancelled {
            do {
                let request = try buildSSERequest(tables: tables, filter: filter)
                let connection = try await sseTransport.connect(request)
                subscription.setConnection(connection)
                var sawValidEvent = false

                for try await message in SseParser(bytes: connection.byteStream()).messages() {
                    if Task.isCancelled {
                        break
                    }
                    do {
                        guard let event = try decodeRealtimeEvent(message: message) else {
                            continue
                        }
                        callbackBox.callback(event)
                        if !sawValidEvent {
                            sawValidEvent = true
                            reconnectAttempt = 0
                        }
                    } catch {
                        // Ignore malformed data events by contract.
                        continue
                    }
                }
                connection.cancel()
                subscription.clearConnection()
            } catch let error as AYBError {
                if error.status == 401 || error.status == 403 {
                    break
                }
            } catch is CancellationError {
                break
            } catch {
                // Continue to reconnect.
            }

            if Task.isCancelled {
                break
            }

            guard reconnectAttempt < options.maxReconnectAttempts else {
                break
            }
            let baseDelay = options.reconnectDelays[min(reconnectAttempt, options.reconnectDelays.count - 1)]
            let jitter = min(max(jitterProvider(options.jitterMax), 0), options.jitterMax)
            reconnectAttempt += 1
            do {
                try await sleep(UInt64((baseDelay + jitter) * 1_000_000_000))
            } catch {
                break
            }
        }
    }

    private func buildSSERequest(tables: [String], filter: String?) throws -> HTTPRequest {
        let base = "\(client.configuration.baseURL)/api/realtime"
        guard var components = URLComponents(string: base) else {
            throw RequestBuilderError.unableToBuildURL
        }

        var queryItems = [URLQueryItem(name: "tables", value: tables.joined(separator: ","))]
        if let token = client.token, !token.isEmpty {
            queryItems.append(URLQueryItem(name: "token", value: token))
        }
        if let filter, !filter.isEmpty {
            queryItems.append(URLQueryItem(name: "filter", value: filter))
        }
        components.queryItems = queryItems

        guard let url = components.url else {
            throw RequestBuilderError.unableToBuildURL
        }

        return HTTPRequest(
            url: url,
            method: .get,
            headers: [
                "Accept": "text/event-stream",
                "Cache-Control": "no-cache",
            ],
            body: nil
        )
    }

    private func decodeRealtimeEvent(message: SseMessage) throws -> RealtimeEvent? {
        if message.event == "connected" {
            return nil
        }
        guard let data = message.data, !data.isEmpty else {
            return nil
        }
        guard let parsed = AYBJSON.parse(Data(data.utf8)) else {
            return nil
        }
        let dictionary = try AYBJSON.expectDictionary(parsed, "RealtimeEvent")
        return try RealtimeEvent(from: dictionary)
    }

    private func requireChannelSubscription(_ channel: String) throws {
        guard wsState.isChannelSubscribed(channel) else {
            throw AYBError(
                status: 400,
                message: "not subscribed to channel",
                code: "realtime/not-subscribed"
            )
        }
    }

    // MARK: WebSocket

    public func connectWebSocket() async throws {
        if wsState.connected() {
            return
        }

        let url = try buildWebSocketURL()
        let connection = try await wsTransport.connect(url: url, headers: [:])
        let handshakeText = try await connection.receive()
        let handshake = try decodeWebSocketServerMessage(handshakeText)
        guard handshake.type == .connected else {
            await connection.close()
            throw AYBError(status: 0, message: "expected connected message")
        }

        let receiveTask = Task<Void, Never> { [weak self] in
            guard let self else { return }
            await self.runWebSocketReceiveLoop(connection: connection)
        }
        let pingTask = Task<Void, Never> { [weak self] in
            guard let self else { return }
            await self.runWebSocketPingLoop(connection: connection)
        }
        wsState.setConnection(connection, receiveTask: receiveTask, pingTask: pingTask)
    }

    public func disconnectWebSocket() {
        let connection = wsState.connectionSnapshot()
        wsState.markDisconnected(AYBError(status: 0, message: "websocket closed"))
        Task {
            await connection?.close()
        }
    }

    public func subscribeWS(
        tables: [String],
        filter: String? = nil,
        callback: @escaping (RealtimeEvent) -> Void
    ) async throws -> () -> Void {
        guard !tables.isEmpty else { return {} }
        try await connectWebSocket()

        let callbackID = UUID()
        wsState.addTableCallback(id: callbackID, tables: Set(tables), callback: callback)
        do {
            _ = try await sendAndAwaitReply(
                WebSocketClientMessage(type: .subscribe, tables: tables, filter: filter)
            )
        } catch {
            wsState.removeTableCallback(id: callbackID)
            throw error
        }

        return { [weak self] in
            guard let self else { return }
            Task {
                self.wsState.removeTableCallback(id: callbackID)
                _ = try? await self.sendAndAwaitReply(
                    WebSocketClientMessage(type: .unsubscribe, tables: tables)
                )
            }
        }
    }

    public func channelSubscribe(_ channel: String) async throws -> () -> Void {
        try await connectWebSocket()
        _ = try await sendAndAwaitReply(
            WebSocketClientMessage(type: .channelSubscribe, channel: channel)
        )
        wsState.addChannelSubscription(channel)
        return { [weak self] in
            guard let self else { return }
            Task {
                self.wsState.removeChannelSubscription(channel)
                _ = try? await self.sendAndAwaitReply(
                    WebSocketClientMessage(type: .channelUnsubscribe, channel: channel)
                )
            }
        }
    }

    public func onBroadcast(
        channel: String,
        callback: @escaping (String, [String: Any]) -> Void
    ) -> () -> Void {
        let callbackID = UUID()
        wsState.addBroadcastCallback(channel: channel, id: callbackID, callback: callback)
        return { [weak self] in
            guard let self else { return }
            self.wsState.removeBroadcastCallback(channel: channel, id: callbackID)
        }
    }

    public func broadcast(
        channel: String,
        event: String,
        payload: [String: Any],
        self includeSelf: Bool = false
    ) async throws {
        try requireChannelSubscription(channel)
        _ = try await sendAndAwaitReply(
            WebSocketClientMessage(
                type: .broadcast,
                channel: channel,
                event: event,
                payload: payload,
                selfBroadcast: includeSelf
            )
        )
    }

    public func presenceTrack(channel: String, state: [String: Any]) async throws {
        try requireChannelSubscription(channel)
        _ = try await sendAndAwaitReply(
            WebSocketClientMessage(type: .presenceTrack, channel: channel, presence: state)
        )
    }

    public func presenceUntrack(channel: String) async throws {
        try requireChannelSubscription(channel)
        _ = try await sendAndAwaitReply(
            WebSocketClientMessage(type: .presenceUntrack, channel: channel)
        )
    }

    public func presenceSync(channel: String) async throws -> [String: Any] {
        try requireChannelSubscription(channel)

        return try await withCheckedThrowingContinuation { continuation in
            wsState.storePresenceSyncContinuation(continuation, channel: channel)
            Task {
                do {
                    _ = try await sendAndAwaitReply(
                        WebSocketClientMessage(type: .presenceSync, channel: channel)
                    )
                } catch {
                    wsState.cancelPresenceSync(channel: channel, error: error)
                }
            }
        }.value
    }

    private func sendAndAwaitReply(_ message: WebSocketClientMessage) async throws -> WebSocketServerMessage {
        try await connectWebSocket()
        let ref = wsState.nextRef()
        let connection = try await requireWebSocketConnection()
        var payload = message.toDictionary()
        payload["ref"] = ref
        let data = try JSONSerialization.data(withJSONObject: payload, options: [])
        let text = String(decoding: data, as: UTF8.self)

        let reply = try await withCheckedThrowingContinuation { continuation in
            Task {
                wsState.storeReplyContinuation(continuation, ref: ref)
                do {
                    try await connection.send(text: text)
                } catch {
                    wsState.failReplyContinuation(ref: ref, error: error)
                }
            }
        }

        if reply.status?.lowercased() == "error" {
            throw AYBError(status: 400, message: reply.message ?? "websocket reply error")
        }

        return reply
    }

    private func requireWebSocketConnection() async throws -> any WebSocketConnection {
        guard let connection = wsState.connectionSnapshot() else {
            throw AYBError(status: 0, message: "websocket not connected")
        }
        return connection
    }

    private func runWebSocketReceiveLoop(connection: any WebSocketConnection) async {
        while !Task.isCancelled {
            do {
                let raw = try await connection.receive()
                let message = try decodeWebSocketServerMessage(raw)
                await handleWebSocketServerMessage(message)
            } catch is CancellationError {
                break
            } catch {
                break
            }
        }
        wsState.markDisconnected(AYBError(status: 0, message: "websocket disconnected"))
        await connection.close()
    }

    private func runWebSocketPingLoop(connection: any WebSocketConnection) async {
        while !Task.isCancelled {
            do {
                try await sleep(UInt64(wsPingInterval * 1_000_000_000))
                try await connection.ping()
            } catch {
                break
            }
        }
    }

    private func handleWebSocketServerMessage(_ message: WebSocketServerMessage) async {
        switch message.type {
        case .reply:
            guard let ref = message.ref else { return }
            wsState.resolveReply(ref: ref, message: message)
        case .event:
            guard
                let action = message.action,
                let table = message.table,
                let record = message.record
            else {
                return
            }
            let event = RealtimeEvent(
                action: action,
                table: table,
                record: record,
                oldRecord: message.oldRecord
            )
            wsState.dispatchTableEvent(event)
        case .broadcast:
            guard
                let channel = message.channel,
                let event = message.event,
                let payload = message.payload
            else {
                return
            }
            wsState.dispatchBroadcast(channel: channel, event: event, payload: payload)
        case .presence:
            guard message.presenceAction == "sync", let channel = message.channel, let presences = message.presences else {
                return
            }
            wsState.resolvePresenceSync(channel: channel, presences: presences)
        case .connected, .error, .system:
            // No-op for now. Presence diff messages can be handled in Stage 4 WS extension work.
            return
        }
    }

    private func buildWebSocketURL() throws -> URL {
        guard var components = URLComponents(url: client.configuration.baseURL, resolvingAgainstBaseURL: false) else {
            throw RequestBuilderError.unableToBuildURL
        }
        if components.scheme == "https" {
            components.scheme = "wss"
        } else if components.scheme == "http" {
            components.scheme = "ws"
        }
        let basePath = components.path.trimmingCharacters(in: CharacterSet(charactersIn: "/"))
        if basePath.isEmpty {
            components.path = "/api/realtime/ws"
        } else {
            components.path = "/\(basePath)/api/realtime/ws"
        }
        if let token = client.token, !token.isEmpty {
            components.queryItems = [URLQueryItem(name: "token", value: token)]
        } else {
            components.queryItems = nil
        }
        guard let url = components.url else {
            throw RequestBuilderError.unableToBuildURL
        }
        return url
    }

    private func decodeWebSocketServerMessage(_ text: String) throws -> WebSocketServerMessage {
        guard let parsed = AYBJSON.parse(Data(text.utf8)) else {
            throw AYBDecodingError.invalidType("WebSocketServerMessage")
        }
        let dictionary = try AYBJSON.expectDictionary(parsed, "WebSocketServerMessage")
        return try WebSocketServerMessage(from: dictionary)
    }
}
