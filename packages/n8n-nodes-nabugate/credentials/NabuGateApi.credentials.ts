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
