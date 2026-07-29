/// Official Dart and Flutter client for NabuGate, the OpenAI-compatible AI
/// gateway.
///
/// The gateway passes request bodies through to the upstream provider
/// untouched, so every method takes an `extra` map alongside its named
/// arguments. Anything placed there reaches the provider as-is, which means a
/// new provider parameter needs no release of this package.
library nabugate_sdk;

import 'dart:async';
import 'dart:convert';

import 'package:http/http.dart' as http;

/// The hosted gateway.
const String kDefaultBaseUrl = 'https://gate.nabuxai.com/v1';

/// A non-2xx response from the gateway.
class NabuGateException implements Exception {
  NabuGateException(this.statusCode, this.body);

  final int statusCode;
  final String body;

  /// The gateway's error code, when the body carried one.
  String? get code {
    try {
      final error = (jsonDecode(body) as Map<String, dynamic>)['error'];
      if (error is Map) return (error['code'] ?? error['message']) as String?;
      if (error is String) return error;
    } catch (_) {
      // A non-JSON body simply has no code to report.
    }
    return null;
  }

  @override
  String toString() => 'NabuGate request failed ($statusCode): $body';
}

/// One chat message. [content] is dynamic so multimodal parts and tool results
/// pass through unchanged.
class Message {
  const Message(
      {required this.role, required this.content, this.name, this.toolCallId});

  const Message.system(String text) : this(role: 'system', content: text);
  const Message.user(String text) : this(role: 'user', content: text);
  const Message.assistant(String text) : this(role: 'assistant', content: text);

  final String role;
  final Object? content;
  final String? name;
  final String? toolCallId;

  Map<String, dynamic> toJson() => {
        'role': role,
        'content': content,
        if (name != null) 'name': name,
        if (toolCallId != null) 'tool_call_id': toolCallId,
      };
}

/// Client for a NabuGate deployment.
class NabuGateClient {
  NabuGateClient({
    required this.apiKey,
    String baseUrl = kDefaultBaseUrl,
    this.defaultModel = 'nabu-smart',
    this.timeout = const Duration(seconds: 120),
    Map<String, String>? headers,
    http.Client? httpClient,
  })  : baseUrl = baseUrl.replaceAll(RegExp(r'/+$'), ''),
        extraHeaders = headers ?? const {},
        _http = httpClient ?? http.Client();

  final String apiKey;
  final String baseUrl;
  final String defaultModel;
  final Duration timeout;
  final Map<String, String> extraHeaders;
  final http.Client _http;

  Map<String, String> get _headers => {
        'Authorization': 'Bearer $apiKey',
        'Content-Type': 'application/json',
        ...extraHeaders,
      };

  /// Performs a chat completion. Entries in [extra] are passed through
  /// untouched.
  Future<Map<String, dynamic>> chat(
    List<Message> messages, {
    String? model,
    double? temperature,
    int? maxTokens,
    String? conversationId,
    Map<String, dynamic> extra = const {},
  }) async {
    final body = _chatBody(
      messages,
      model: model,
      temperature: temperature,
      maxTokens: maxTokens,
      conversationId: conversationId,
      extra: extra,
      stream: false,
    );
    final response = await _post('/chat/completions', body);
    return jsonDecode(response.body) as Map<String, dynamic>;
  }

  /// Performs a chat completion and returns only the assistant's text.
  Future<String> completeText(
    List<Message> messages, {
    String? model,
    double? temperature,
    int? maxTokens,
    Map<String, dynamic> extra = const {},
  }) async {
    final data = await chat(
      messages,
      model: model,
      temperature: temperature,
      maxTokens: maxTokens,
      extra: extra,
    );
    final choices = data['choices'] as List?;
    if (choices == null || choices.isEmpty) return '';
    final content = (choices.first as Map)['message']?['content'];
    return content is String ? content.trim() : '';
  }

  /// Streams a chat completion, yielding text deltas as they arrive.
  Stream<String> stream(
    List<Message> messages, {
    String? model,
    double? temperature,
    Map<String, dynamic> extra = const {},
  }) async* {
    await for (final chunk in streamChunks(
      messages,
      model: model,
      temperature: temperature,
      extra: extra,
    )) {
      final choices = chunk['choices'] as List?;
      if (choices == null || choices.isEmpty) continue;
      final delta = (choices.first as Map)['delta']?['content'];
      if (delta is String && delta.isNotEmpty) yield delta;
    }
  }

  /// Streams a chat completion, yielding whole SSE payloads. Use this when you
  /// need tool-call deltas or finish reasons rather than plain text.
  Stream<Map<String, dynamic>> streamChunks(
    List<Message> messages, {
    String? model,
    double? temperature,
    Map<String, dynamic> extra = const {},
  }) async* {
    final body = _chatBody(
      messages,
      model: model,
      temperature: temperature,
      extra: extra,
      stream: true,
    );
    final request = http.Request('POST', Uri.parse('$baseUrl/chat/completions'))
      ..headers.addAll(_headers)
      ..body = jsonEncode(body);

    // No timeout on a stream: a long generation is the normal case, and
    // aborting mid-stream throws away tokens that were already paid for.
    final response = await _http.send(request);
    if (response.statusCode >= 400) {
      throw NabuGateException(
          response.statusCode, await response.stream.bytesToString());
    }

    final lines =
        response.stream.transform(utf8.decoder).transform(const LineSplitter());
    await for (final line in lines) {
      final trimmed = line.trim();
      if (!trimmed.startsWith('data:')) continue;
      final payload = trimmed.substring(5).trim();
      if (payload == '[DONE]') return;
      try {
        yield jsonDecode(payload) as Map<String, dynamic>;
      } catch (_) {
        // Skip an unparseable payload rather than killing the stream.
      }
    }
  }

  /// Creates embeddings.
  ///
  /// Pass [dimensions] whenever the vectors are being stored: a consumer that
  /// writes to a fixed-width column cannot accept whatever width the provider
  /// happens to default to. Leave it null for ad-hoc search, since providers
  /// without the field reject it.
  Future<Map<String, dynamic>> embeddings(
    Object input, {
    String model = 'nabu-embed',
    int? dimensions,
    Map<String, dynamic> extra = const {},
  }) async {
    final response = await _post('/embeddings', {
      'model': model,
      'input': input,
      if (dimensions != null) 'dimensions': dimensions,
      ...extra,
    });
    return jsonDecode(response.body) as Map<String, dynamic>;
  }

  /// Generates images. Results carry base64 data in `data[].b64_json`.
  Future<Map<String, dynamic>> images(
    String prompt, {
    String model = 'nabu-image',
    Map<String, dynamic> extra = const {},
  }) async {
    final response = await _post('/images/generations', {
      'model': model,
      'prompt': prompt,
      ...extra,
    });
    return jsonDecode(response.body) as Map<String, dynamic>;
  }

  /// Synthesises speech and returns the raw audio bytes.
  Future<List<int>> speech(
    String input, {
    String model = 'nabu-voice',
    String? voice,
    Map<String, dynamic> extra = const {},
  }) async {
    final response = await _post('/audio/speech', {
      'model': model,
      'input': input,
      if (voice != null) 'voice': voice,
      ...extra,
    });
    return response.bodyBytes;
  }

  /// Lists every model, alias and agent this key may call.
  Future<List<Map<String, dynamic>>> models() async {
    final response = await _get('/models');
    final data =
        (jsonDecode(response.body) as Map<String, dynamic>)['data'] as List?;
    return (data ?? []).cast<Map<String, dynamic>>();
  }

  /// Returns token and cost usage for this key.
  Future<Map<String, dynamic>> usage() async {
    final response = await _get('/usage');
    return jsonDecode(response.body) as Map<String, dynamic>;
  }

  /// Releases the underlying HTTP client.
  void close() => _http.close();

  // -------------------------------------------------------------- internals

  Map<String, dynamic> _chatBody(
    List<Message> messages, {
    String? model,
    double? temperature,
    int? maxTokens,
    String? conversationId,
    Map<String, dynamic> extra = const {},
    required bool stream,
  }) =>
      {
        'model': model ?? defaultModel,
        'messages': messages.map((m) => m.toJson()).toList(),
        if (temperature != null) 'temperature': temperature,
        if (maxTokens != null) 'max_tokens': maxTokens,
        if (conversationId != null) 'conversation_id': conversationId,
        ...extra,
        'stream': stream,
      };

  Future<http.Response> _post(String path, Map<String, dynamic> body) async {
    final response = await _http
        .post(Uri.parse('$baseUrl$path'),
            headers: _headers, body: jsonEncode(body))
        .timeout(timeout);
    return _check(response);
  }

  Future<http.Response> _get(String path) async {
    final response = await _http
        .get(Uri.parse('$baseUrl$path'), headers: _headers)
        .timeout(timeout);
    return _check(response);
  }

  http.Response _check(http.Response response) {
    if (response.statusCode >= 400) {
      throw NabuGateException(response.statusCode, response.body);
    }
    return response;
  }
}
