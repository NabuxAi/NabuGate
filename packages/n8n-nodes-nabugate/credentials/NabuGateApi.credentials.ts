import {
	ICredentialType,
	INodeProperties,
} from 'n8n-workflow';

export class NabuGateApi implements ICredentialType {
	name = 'nabuGateApi';
	displayName = 'NabuGate API';
	documentationUrl = 'https://github.com/nabuxai/NabuGate#readme';
	properties: INodeProperties[] = [
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
