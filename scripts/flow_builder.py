#!/usr/bin/env python3
"""
NabuGate Flow & Agent Builder CLI
A simple tool to create new agents and connect them into multi-agent pipelines (Flows).

Usage Examples:
  # List all available agents and flows
  python3 scripts/flow_builder.py list

  # Create a single agent
  python3 scripts/flow_builder.py create-agent \
      --name translator \
      --desc "Translates English text to fluent Persian" \
      --model nabu-fast \
      --system "Translate the provided text to fluent, natural Persian."

  # Connect agents into a Flow
  python3 scripts/flow_builder.py create-flow \
      --name content-refiner \
      --desc "Drafts content, critiques it, and produces a final version" \
      --agents sales-reader,sales-drafter,sales-reviewer

  # Interactive wizard mode
  python3 scripts/flow_builder.py interactive
"""

import argparse
import glob
import os
import sys
import yaml

REPO_ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
AGENTS_DIR = os.path.join(REPO_ROOT, "agents")
FLOWS_DIR = os.path.join(REPO_ROOT, "flows")

def list_items():
    print("=" * 60)
    print("🤖 NabuGate Available Sub-Agents:")
    print("=" * 60)
    agent_files = sorted(glob.glob(os.path.join(AGENTS_DIR, "*.yaml")))
    if not agent_files:
        print("  (No agents found)")
    for f in agent_files:
        name = os.path.splitext(os.path.basename(f))[0]
        try:
            with open(f, "r", encoding="utf-8") as yf:
                data = yaml.safe_load(yf) or {}
                desc = data.get("description", "No description")
                model = data.get("model", "default")
                print(f"  • \033[1;32m{name}\033[0m (model: {model})")
                print(f"    {desc}")
        except Exception:
            print(f"  • {name}")

    print("\n" + "=" * 60)
    print("🔄 NabuGate Multi-Agent Flows:")
    print("=" * 60)
    flow_files = sorted(glob.glob(os.path.join(FLOWS_DIR, "*.yaml")))
    if not flow_files:
        print("  (No flows found)")
    for f in flow_files:
        name = os.path.splitext(os.path.basename(f))[0]
        try:
            with open(f, "r", encoding="utf-8") as yf:
                data = yaml.safe_load(yf) or {}
                desc = data.get("description", "No description")
                steps = data.get("steps", [])
                step_agents = " ➔ ".join(s.get("agent", "?") for s in steps)
                print(f"  • \033[1;36m{name}\033[0m: {step_agents}")
                print(f"    {desc}")
        except Exception:
            print(f"  • {name}")
    print()

def create_agent(name, description, system, model="nabu-smart", temperature=0.3, max_tokens=4096):
    name = name.strip().lower().replace(" ", "-")
    filepath = os.path.join(AGENTS_DIR, f"{name}.yaml")
    
    agent_data = {
        "name": name,
        "description": description.strip(),
        "model": model.strip(),
        "temperature": float(temperature),
        "max_tokens": int(max_tokens),
        "system": system.strip() + "\n"
    }

    os.makedirs(AGENTS_DIR, exist_ok=True)
    with open(filepath, "w", encoding="utf-8") as f:
        yaml.dump(agent_data, f, allow_unicode=True, sort_keys=False, default_flow_style=False)

    print(f"\033[1;32m[+] Agent '{name}' created successfully at:\033[0m {filepath}")
    return filepath

def create_flow(name, description, agent_names, reviewer_prompt=None):
    name = name.strip().lower().replace(" ", "-")
    filepath = os.path.join(FLOWS_DIR, f"{name}.yaml")

    steps = []
    for i, agent in enumerate(agent_names):
        agent = agent.strip()
        if not agent:
            continue
        step = {
            "agent": agent,
            "label": f"Step {i+1}: {agent}"
        }
        # If it's the last step in a multi-agent chain, provide smart reviewer input if requested
        if i == len(agent_names) - 1 and len(agent_names) > 1:
            if reviewer_prompt:
                step["input"] = reviewer_prompt.strip() + "\n"
            else:
                step["input"] = "درخواست اولیه کاربر:\n{{input}}\n\nپیش‌نویس و تحلیل مراحل قبل:\n{{previous}}\n\nاین خروجی را بررسی و نسخهٔ نهایی بدون نقص را برگردان.\n"
        
        steps.append(step)

    flow_data = {
        "name": name,
        "description": description.strip(),
        "steps": steps
    }

    os.makedirs(FLOWS_DIR, exist_ok=True)
    with open(filepath, "w", encoding="utf-8") as f:
        yaml.dump(flow_data, f, allow_unicode=True, sort_keys=False, default_flow_style=False)

    print(f"\033[1;32m[+] Flow '{name}' created successfully at:\033[0m {filepath}")
    print(f"    Pipeline: {' ➔ '.join(agent_names)}")
    return filepath

def interactive_wizard():
    print("\n🧙 NabuGate Interactive Agent & Flow Wizard\n")
    print("1. List existing agents and flows")
    print("2. Create a new Sub-Agent")
    print("3. Connect agents into a Multi-Agent Flow")
    print("4. Exit")
    
    choice = input("\nSelect an option (1-4): ").strip()
    if choice == "1":
        list_items()
    elif choice == "2":
        name = input("Agent Name (e.g. text-summarizer): ").strip()
        desc = input("Description: ").strip()
        model = input("Model alias [nabu-smart / nabu-fast / nabu-cheap] (default: nabu-smart): ").strip() or "nabu-smart"
        print("System instructions (press Enter then Ctrl+D or type end on a new line):")
        lines = []
        while True:
            try:
                line = input()
                if line.strip() == "end":
                    break
                lines.append(line)
            except EOFError:
                break
        system = "\n".join(lines)
        create_agent(name, desc, system, model=model)
    elif choice == "3":
        name = input("Flow Name (e.g. summarize-and-review): ").strip()
        desc = input("Flow Description: ").strip()
        agents_raw = input("Agent sequence (comma-separated, e.g. agent1,agent2,agent3): ").strip()
        agent_list = [a.strip() for a in agents_raw.split(",") if a.strip()]
        create_flow(name, desc, agent_list)
    else:
        print("Goodbye!")

def main():
    parser = argparse.ArgumentParser(description="NabuGate Agent & Flow Builder")
    subparsers = parser.add_subparsers(dest="command", help="Command to run")

    # list
    subparsers.add_parser("list", help="List all agents and flows")

    # create-agent
    p_agent = subparsers.add_parser("create-agent", help="Create a new sub-agent YAML")
    p_agent.add_argument("--name", required=True, help="Agent unique name")
    p_agent.add_argument("--desc", required=True, help="Human-readable description")
    p_agent.add_argument("--system", required=True, help="System prompt")
    p_agent.add_argument("--model", default="nabu-smart", help="Model alias (nabu-smart, nabu-fast, etc.)")
    p_agent.add_argument("--temp", default=0.3, type=float, help="Temperature (default: 0.3)")
    p_agent.add_argument("--max-tokens", default=4096, type=int, help="Max tokens (default: 4096)")

    # create-flow
    p_flow = subparsers.add_parser("create-flow", help="Connect agents into a flow YAML")
    p_flow.add_argument("--name", required=True, help="Flow unique name")
    p_flow.add_argument("--desc", required=True, help="Flow description")
    p_flow.add_argument("--agents", required=True, help="Comma-separated agent names")
    p_flow.add_argument("--prompt", help="Custom prompt template for the final reviewer step")

    # interactive
    subparsers.add_parser("interactive", help="Run interactive wizard")

    args = parser.parse_args()

    if args.command == "list":
        list_items()
    elif args.command == "create-agent":
        create_agent(args.name, args.desc, args.system, model=args.model, temperature=args.temp, max_tokens=args.max_tokens)
    elif args.command == "create-flow":
        agent_names = [a.strip() for a in args.agents.split(",") if a.strip()]
        create_flow(args.name, args.desc, agent_names, reviewer_prompt=args.prompt)
    elif args.command == "interactive" or not args.command:
        interactive_wizard()

if __name__ == "__main__":
    main()
