"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.NabuGateApi = void 0;

class NabuGateApi {
    constructor() {
        this.name = 'nabuGateApi';
        this.displayName = 'NabuGate API';
        this.documentationUrl = 'https://github.com/nabuxai/NabuGate#readme';
        this.properties = [
            {
                displayName: 'Base URL',
                name: 'baseUrl',
                type: 'string',
                default: 'http://localhost:8080/v1',
                description: 'Base URL of your NabuGate instance (e.g. http://localhost:8080/v1 or https://gateway.nabuxai.com/v1)',
                required: true,
            },
            {
                displayName: 'API Key',
                name: 'apiKey',
                type: 'string',
                typeOptions: {
                    password: true,
                },
                default: '',
                description: 'The API key or Project key configured in NabuGate',
                required: true,
            },
        ];
    }
}
exports.NabuGateApi = NabuGateApi;
