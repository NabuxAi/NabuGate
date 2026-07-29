<?php

namespace NabuGate\Middleware;

use Closure;
use Illuminate\Http\Request;
use Symfony\Component\HttpFoundation\Response;

class NabuGateMeteringMiddleware
{
    public function handle(Request $request, Closure $next): Response
    {
        $user = $request->user();

        // 1. Verify user wallet has minimum balance
        if ($user && isset($user->wallet_balance_cents) && $user->wallet_balance_cents <= 0) {
            return response()->json([
                'error' => [
                    'message' => 'Insufficient wallet balance. Please top up your NabuAuth account.',
                    'code' => 'insufficient_funds',
                ]
            ], 402);
        }

        $response = $next($request);

        // 2. Extract token usage and debit wallet
        if ($response->isSuccessful() && $user) {
            $content = json_decode($response->getContent(), true);
            $totalTokens = (int) data_get($content, 'usage.total_tokens', 0);

            if ($totalTokens > 0) {
                // Rate: 1 baisa per 100 tokens
                $costCents = (int) ceil($totalTokens / 100);
                if (method_exists($user, 'debitWallet')) {
                    $user->debitWallet($costCents, "NabuGate LLM completion ({$totalTokens} tokens)");
                }
            }
        }

        return $response;
    }
}
