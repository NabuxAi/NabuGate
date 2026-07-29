import 'dart:convert';
import 'package:http/http.dart' as http;

class NabuGateClient {
  NabuGateClient({
    required this.apiKey,
    this.baseUrl = 'https://gate.nabuxai.com/v1',
    this.defaultModel = 'nabu-smart',
  });

  final String apiKey;
  final String baseUrl;
  final String defaultModel;

  Future<Map<String, dynamic>> chat(
    List<Map<String, String>> messages, {
    String? model,
    double temperature = 0.7,
  }) async {
    final uri = Uri.parse('${baseUrl.replaceAll(RegExp(r'/$'), '')}/chat/completions');
    final response = await http.post(
      uri,
      headers: {
        'Authorization': 'Bearer $apiKey',
        'Content-Type': 'application/json',
      },
      body: jsonEncode({
        'model': model ?? defaultModel,
        'messages': messages,
        'temperature': temperature,
      }),
    );

    if (response.statusCode != 200) {
      throw Exception('NabuGate request failed (${response.statusCode}): ${response.body}');
    }

    return jsonDecode(response.body) as Map<String, dynamic>;
  }

  Future<String> completeText(
    List<Map<String, String>> messages, {
    String? model,
    double temperature = 0.7,
  }) async {
    final data = await chat(messages, model: model, temperature: temperature);
    final choices = data['choices'] as List?;
    if (choices != null && choices.isNotEmpty) {
      final msg = choices[0]['message'] as Map?;
      return (msg?['content'] as String? ?? '').trim();
    }
    return '';
  }
}
