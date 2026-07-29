<?php

namespace NabuGate\Facades;

use Illuminate\Support\Facades\Facade;

/**
 * @method static \NabuGate\Client\NabuGateClient client()
 * @method static \NabuGate\Services\StoryWriter story()
 */
class NabuGate extends Facade
{
    protected static function getFacadeAccessor(): string
    {
        return 'nabugate';
    }
}
