import {
	IExecuteFunctions,
	IDataObject,
	INodeExecutionData,
	INodeType,
	INodeTypeDescription,
	NodeOperationError,
} from 'n8n-workflow';

export class NabuGate implements INodeType {
	description: INodeTypeDescription = {
		displayName: 'NabuGate',
		name: 'nabuGate',
		icon: 'file:nabugate.svg',
		group: ['transform'],
		version: 1,
		subtitle: '={{$parameter["operation"] + ": " + $parameter["resource"]}}',
		description: 'Interact with NabuGate AI gateway, sub-agents, and multi-agent flows',
		defaults: {
			name: 'NabuGate',
		},
		inputs: ['main'],
		outputs: ['main'],
		credentials: [
			{
				name: 'nabuGateApi',
				required: true,
			},
		],
		properties: [
			// Resource
			{
				displayName: 'Resource',
				name: 'resource',
				type: 'options',
				noDataExpression: true,
				options: [
					{
						name: 'Agent / Flow Execution',
						value: 'agent',
					},
					{
						name: 'Embeddings',
						value: 'embedding',
					},
					{
						name: 'Image Generation',
						value: 'image',
					},
					{
						name: 'Speech (Audio)',
						value: 'audio',
					},
					{
						name: 'Model & Agent Registry',
						value: 'models',
					},
				],
				default: 'agent',
			},

			// Operations for Agent / Flow
			{
				displayName: 'Operation',
				name: 'operation',
				type: 'options',
				noDataExpression: true,
				displayOptions: {
					show: {
						resource: ['agent'],
					},
				},
				options: [
					{
						name: 'Execute Multi-Agent Flow',
						value: 'executeFlow',
						description: 'Run a multi-agent sequence (e.g. seo-audit-team, sales-team)',
						action: 'Execute a multi agent flow',
					},
					{
						name: 'Run Sub-Agent',
						value: 'runAgent',
						description: 'Call a specialized sub-agent (e.g. seo-content-auditor, agent-architect)',
						action: 'Run a specialized sub agent',
					},
					{
						name: 'Standard Chat Completion',
						value: 'chatCompletion',
						description: 'Send a prompt to a chat alias (e.g. nabu-smart, nabu-fast)',
						action: 'Send standard chat completion',
					},
				],
				default: 'executeFlow',
			},

			// Fields for Execute Multi-Agent Flow
			{
				displayName: 'Flow Name or ID',
				name: 'flowName',
				type: 'options',
				typeOptions: {
					loadOptionsMethod: 'getFlows',
				},
				displayOptions: {
					show: {
						resource: ['agent'],
						operation: ['executeFlow'],
					},
				},
				default: 'seo-audit-team',
				description: 'Select the multi-agent flow to execute. Choose from the list, or specify an ID using an <a href="https://docs.n8n.io/code-examples/expressions/">expression</a>.',
				options: [
					{ name: 'SEO Audit Team (seo-audit-team)', value: 'seo-audit-team' },
					{ name: 'Sales Team (sales-team)', value: 'sales-team' },
				],
			},
			{
				displayName: 'Input Content / Article',
				name: 'prompt',
				type: 'string',
				typeOptions: {
					rows: 6,
				},
				displayOptions: {
					show: {
						resource: ['agent'],
						operation: ['executeFlow', 'runAgent', 'chatCompletion'],
					},
				},
				default: '',
				required: true,
				description: 'The input message, article text, or prompt to process',
			},

			// Fields for Run Sub-Agent
			{
				displayName: 'Agent Name',
				name: 'agentName',
				type: 'options',
				displayOptions: {
					show: {
						resource: ['agent'],
						operation: ['runAgent'],
					},
				},
				default: 'seo-content-auditor',
				options: [
					{ name: 'SEO Content Auditor (seo-content-auditor)', value: 'seo-content-auditor' },
					{ name: 'SEO Schema Engineer (seo-schema-engineer)', value: 'seo-schema-engineer' },
					{ name: 'SEO Strategist Reviewer (seo-strategist-reviewer)', value: 'seo-strategist-reviewer' },
					{ name: 'Agent Architect (agent-architect)', value: 'agent-architect' },
					{ name: 'Sales Drafter (sales-drafter)', value: 'sales-drafter' },
					{ name: 'Sales Reviewer (sales-reviewer)', value: 'sales-reviewer' },
					{ name: 'Write Composer (write-composer)', value: 'write-composer' },
					{ name: 'Write Editor (write-editor)', value: 'write-editor' },
				],
				description: 'Select which sub-agent to invoke',
			},

			// Fields for Chat Completion
			{
				displayName: 'Model Alias',
				name: 'modelAlias',
				type: 'options',
				displayOptions: {
					show: {
						resource: ['agent'],
						operation: ['chatCompletion'],
					},
				},
				default: 'nabu-smart',
				options: [
					{ name: 'Nabu Smart (Primary: Gemini 2.5 Pro / Claude 3.7 / GPT-4o)', value: 'nabu-smart' },
					{ name: 'Nabu Fast (Primary: Gemini 2.5 Flash / Groq)', value: 'nabu-fast' },
					{ name: 'Nabu Cheap (Flash-Lite / Qwen)', value: 'nabu-cheap' },
				],
				description: 'The routing alias to query',
			},

			// Common options for Agent / Chat
			{
				displayName: 'System Prompt (Optional)',
				name: 'systemPrompt',
				type: 'string',
				typeOptions: {
					rows: 3,
				},
				displayOptions: {
					show: {
						resource: ['agent'],
						operation: ['chatCompletion'],
					},
				},
				default: '',
				description: 'Override or prepend instructions for the model',
			},
			{
				displayName: 'Temperature',
				name: 'temperature',
				type: 'number',
				typeOptions: {
					minValue: 0,
					maxValue: 2,
					numberStepSize: 0.1,
				},
				displayOptions: {
					show: {
						resource: ['agent'],
					},
				},
				default: 0.3,
				description: 'Sampling temperature between 0 and 2',
			},

			// Operations for Embeddings
			{
				displayName: 'Operation',
				name: 'operation',
				type: 'options',
				noDataExpression: true,
				displayOptions: {
					show: {
						resource: ['embedding'],
					},
				},
				options: [
					{
						name: 'Create Embedding',
						value: 'createEmbedding',
						action: 'Create text embeddings',
					},
				],
				default: 'createEmbedding',
			},
			{
				displayName: 'Text Input',
				name: 'embeddingInput',
				type: 'string',
				typeOptions: {
					rows: 4,
				},
				displayOptions: {
					show: {
						resource: ['embedding'],
					},
				},
				default: '',
				required: true,
				description: 'Text to generate embeddings for',
			},
			{
				displayName: 'Embedding Alias',
				name: 'embeddingModel',
				type: 'options',
				displayOptions: {
					show: {
						resource: ['embedding'],
					},
				},
				default: 'nabu-embed',
				options: [
					{ name: 'Nabu Embed (nabu-embed)', value: 'nabu-embed' },
					{ name: 'Write Embed (write-embed)', value: 'write-embed' },
					{ name: 'Desk Embed (desk-embed)', value: 'desk-embed' },
					{ name: 'Chat Embed (chat-embed)', value: 'chat-embed' },
				],
			},

			// Operations for Image
			{
				displayName: 'Operation',
				name: 'operation',
				type: 'options',
				noDataExpression: true,
				displayOptions: {
					show: {
						resource: ['image'],
					},
				},
				options: [
					{
						name: 'Generate Image',
						value: 'generateImage',
						action: 'Generate an image from prompt',
					},
				],
				default: 'generateImage',
			},
			{
				displayName: 'Image Prompt',
				name: 'imagePrompt',
				type: 'string',
				displayOptions: {
					show: {
						resource: ['image'],
					},
				},
				default: '',
				required: true,
				description: 'Description of the desired image',
			},
			{
				displayName: 'Image Size',
				name: 'imageSize',
				type: 'options',
				displayOptions: {
					show: {
						resource: ['image'],
					},
				},
				default: '1024x1024',
				options: [
					{ name: '1024x1024 (Square)', value: '1024x1024' },
					{ name: '1024x1792 (Portrait)', value: '1024x1792' },
					{ name: '1792x1024 (Landscape)', value: '1792x1024' },
				],
			},

			// Operations for Audio / Speech
			{
				displayName: 'Operation',
				name: 'operation',
				type: 'options',
				noDataExpression: true,
				displayOptions: {
					show: {
						resource: ['audio'],
					},
				},
				options: [
					{
						name: 'Generate Speech',
						value: 'generateSpeech',
						action: 'Convert text to spoken audio',
					},
				],
				default: 'generateSpeech',
			},
			{
				displayName: 'Text to Speak',
				name: 'audioText',
				type: 'string',
				typeOptions: {
					rows: 3,
				},
				displayOptions: {
					show: {
						resource: ['audio'],
					},
				},
				default: '',
				required: true,
				description: 'The text to convert to speech',
			},

			// Operations for Models Registry
			{
				displayName: 'Operation',
				name: 'operation',
				type: 'options',
				noDataExpression: true,
				displayOptions: {
					show: {
						resource: ['models'],
					},
				},
				options: [
					{
						name: 'List All Models & Agents',
						value: 'listModels',
						action: 'List all available models agents and flows',
					},
				],
				default: 'listModels',
			},
		],
	};

	async execute(this: IExecuteFunctions): Promise<INodeExecutionData[][]> {
		const items = this.getInputData();
		const returnData: INodeExecutionData[] = [];

		const credentials = await this.getCredentials('nabuGateApi');
		const baseUrl = ((credentials.baseUrl as string) || 'http://localhost:8080/v1').replace(/\/+$/, '');
		const apiKey = credentials.apiKey as string;

		for (let i = 0; i < items.length; i++) {
			try {
				const resource = this.getNodeParameter('resource', i) as string;
				const operation = this.getNodeParameter('operation', i) as string;

				let responseData: any;

				if (resource === 'agent') {
					let targetModel = '';
					if (operation === 'executeFlow') {
						targetModel = this.getNodeParameter('flowName', i) as string;
					} else if (operation === 'runAgent') {
						targetModel = this.getNodeParameter('agentName', i) as string;
					} else {
						targetModel = this.getNodeParameter('modelAlias', i) as string;
					}

					const prompt = this.getNodeParameter('prompt', i) as string;
					const temperature = this.getNodeParameter('temperature', i, 0.3) as number;

					const messages: any[] = [];
					if (operation === 'chatCompletion') {
						const systemPrompt = this.getNodeParameter('systemPrompt', i, '') as string;
						if (systemPrompt) {
							messages.push({ role: 'system', content: systemPrompt });
						}
					}
					messages.push({ role: 'user', content: prompt });

					const body: IDataObject = {
						model: targetModel,
						messages,
						temperature,
					};

					responseData = await this.helpers.httpRequest({
						method: 'POST',
						url: `${baseUrl}/chat/completions`,
						headers: {
							Authorization: `Bearer ${apiKey}`,
							'Content-Type': 'application/json',
						},
						body,
						json: true,
					});

					// Extract convenient shortcuts
					const content = responseData?.choices?.[0]?.message?.content || '';
					responseData = {
						content,
						raw: responseData,
						modelUsed: targetModel,
					};
				} else if (resource === 'embedding') {
					const input = this.getNodeParameter('embeddingInput', i) as string;
					const model = this.getNodeParameter('embeddingModel', i) as string;

					responseData = await this.helpers.httpRequest({
						method: 'POST',
						url: `${baseUrl}/embeddings`,
						headers: {
							Authorization: `Bearer ${apiKey}`,
							'Content-Type': 'application/json',
						},
						body: {
							input,
							model,
						},
						json: true,
					});
				} else if (resource === 'image') {
					const prompt = this.getNodeParameter('imagePrompt', i) as string;
					const size = this.getNodeParameter('imageSize', i, '1024x1024') as string;

					responseData = await this.helpers.httpRequest({
						method: 'POST',
						url: `${baseUrl}/images/generations`,
						headers: {
							Authorization: `Bearer ${apiKey}`,
							'Content-Type': 'application/json',
						},
						body: {
							prompt,
							size,
						},
						json: true,
					});
				} else if (resource === 'audio') {
					const input = this.getNodeParameter('audioText', i) as string;

					responseData = await this.helpers.httpRequest({
						method: 'POST',
						url: `${baseUrl}/audio/speech`,
						headers: {
							Authorization: `Bearer ${apiKey}`,
							'Content-Type': 'application/json',
						},
						body: {
							input,
							model: 'tts-1',
						},
						json: true,
					});
				} else if (resource === 'models') {
					responseData = await this.helpers.httpRequest({
						method: 'GET',
						url: `${baseUrl}/models`,
						headers: {
							Authorization: `Bearer ${apiKey}`,
						},
						json: true,
					});
				}

				returnData.push({
					json: responseData,
					pairedItem: { item: i },
				});
			} catch (error: any) {
				if (this.continueOnFail()) {
					returnData.push({
						json: { error: error.message },
						pairedItem: { item: i },
					});
					continue;
				}
				throw new NodeOperationError(this.getNode(), error, { itemIndex: i });
			}
		}

		return [returnData];
	}
}
