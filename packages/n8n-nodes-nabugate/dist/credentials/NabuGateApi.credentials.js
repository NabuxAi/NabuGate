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
                default: 'https://gate.nabuxai.com/v1',
                description: 'Base URL of your NabuGate gateway instance (e.g. https://gate.nabuxai.com/v1 or http://localhost:8080/v1)',
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
                description: 'The API key or Project token configured in NabuGate',
                required: true,
            },
        ];
    }
}
exports.NabuGateApi = NabuGateApi;
