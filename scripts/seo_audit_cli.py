#!/usr/bin/env python3
"""
NabuGate SEO Audit CLI Runner
Evaluates WordPress articles using the NabuGate `seo-audit-team` multi-agent pipeline.

Usage:
  python3 scripts/seo_audit_cli.py --url https://mrchatgpt.org/blog/hell-grind-first-ai-feature-film-higgsfield/
  python3 scripts/seo_audit_cli.py --post-id 13300 --output docs/seo-reports/13300-report.md
"""

import argparse
import json
import os
import sys
import urllib.request
import urllib.parse
from datetime import datetime

DEFAULT_GATEWAY_URL = os.environ.get("NABUGATE_URL", "http://localhost:8080/v1/chat/completions")
NABUGATE_KEY = os.environ.get("NABUGATE_KEY", os.environ.get("NABU_KEY", "nabu-local-key"))

def fetch_url_content(url: str) -> str:
    req = urllib.request.Request(
        url,
        headers={"User-Agent": "Mozilla/5.0 (compatible; NabuGateSEOAuditor/1.0)"}
    )
    with urllib.request.urlopen(req, timeout=15) as response:
        return response.read().decode("utf-8")

def call_seo_audit_team(article_text: str, gateway_url: str = DEFAULT_GATEWAY_URL, api_key: str = NABUGATE_KEY) -> str:
    payload = {
        "model": "seo-audit-team",
        "messages": [
            {
                "role": "user",
                "content": f"لطفاً مقاله زیر را به صورت کامل تحلیل سئو کرده، اسکیماهای استاندارد JSON-LD را بساز و گزارش رتبه‌بندی نهایی را تدوین کن:\n\n{article_text}"
            }
        ]
    }
    data = json.dumps(payload).encode("utf-8")
    req = urllib.request.Request(
        gateway_url,
        data=data,
        headers={
            "Content-Type": "application/json",
            "Authorization": f"Bearer {api_key}"
        }
    )
    with urllib.request.urlopen(req, timeout=120) as response:
        result = json.loads(response.read().decode("utf-8"))
        return result["choices"][0]["message"]["content"]

def main():
    parser = argparse.ArgumentParser(description="NabuGate SEO Audit Multi-Agent Runner")
    parser.add_argument("--url", help="URL of the article to audit")
    parser.add_argument("--file", help="Local HTML/Text file to audit")
    parser.add_argument("--post-id", help="WordPress post ID to audit")
    parser.add_argument("--output", help="Path to save the generated Markdown report")
    parser.add_argument("--gateway-url", default=DEFAULT_GATEWAY_URL, help="NabuGate completions endpoint URL")
    
    args = parser.parse_args()

    content = ""
    target_name = ""

    if args.url:
        print(f"[*] Fetching content from URL: {args.url}")
        content = fetch_url_content(args.url)
        target_name = urllib.parse.urlparse(args.url).path.strip("/").split("/")[-1] or "article"
    elif args.file:
        print(f"[*] Reading content from file: {args.file}")
        with open(args.file, "r", encoding="utf-8") as f:
            content = f.read()
        target_name = os.path.splitext(os.path.basename(args.file))[0]
    elif args.post_id:
        print(f"[*] Auditing Post ID: {args.post_id}")
        target_name = f"post-{args.post_id}"
        # Fallback or prompt for content
        content = f"WordPress Post ID: {args.post_id}"
    else:
        parser.print_help()
        sys.exit(1)

    print(f"[*] Sending article content ({len(content)} bytes) to NabuGate `seo-audit-team`...")
    try:
        report = call_seo_audit_team(content[:15000], gateway_url=args.gateway_url)
        print("[+] SEO Audit completed successfully!")
        
        output_file = args.output
        if not output_file:
            os.makedirs("docs/seo-reports", exist_ok=True)
            timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
            output_file = f"docs/seo-reports/{target_name}_{timestamp}.md"
            
        os.makedirs(os.path.dirname(output_file), exist_ok=True)
        with open(output_file, "w", encoding="utf-8") as f:
            f.write(report)
        print(f"[+] Report saved to: {output_file}")
        print("\n--- Summary Preview ---")
        print(report[:800] + ("..." if len(report) > 800 else ""))
        
    except Exception as e:
        print(f"[-] Error calling NabuGate SEO Audit Team: {e}", file=sys.stderr)
        sys.exit(1)

if __name__ == "__main__":
    main()
