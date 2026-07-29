<?php

namespace NabuGate;

use Illuminate\Support\ServiceProvider;
use NabuGate\Client\NabuGateClient;
use NabuGate\Services\StoryWriter;

class NabuGateServiceProvider extends ServiceProvider
{
    public function register(): void
    {
        $this->mergeConfigFrom(__DIR__ . '/../config/nabugate.php', 'nabugate');

        $this->app->singleton(NabuGateClient::class, function ($app) {
            $config = $app['config']->get('nabugate');
            return new NabuGateClient(
                baseUrl: $config['base_url'] ?? 'https://gate.nabuxai.com/v1',
                apiKey: $config['api_key'] ?? '',
                defaultModel: $config['default_model'] ?? 'nabu-smart',
            );
        });

        $this->app->singleton(StoryWriter::class, function ($app) {
            return new StoryWriter($app->make(NabuGateClient::class));
        });

        $this->app->alias(NabuGateClient::class, 'nabugate');
    }

    public function boot(): void
    {
        if ($this->app->runningInConsole()) {
            $this->publishes([
                __DIR__ . '/../config/nabugate.php' => config_path('nabugate.php'),
            ], 'nabugate-config');
        }
    }
}
