#!/usr/bin/env python3
"""Import DSH's third-party provider catalog into an independent CPA plugin registry.

This does NOT import DSH's anonymous gateway, browser identity, EAC or account-login logic.
Every generated entry is disabled until explicitly reviewed and approved.
"""
from __future__ import annotations
import argparse
import json
import re
from pathlib import Path
from urllib.parse import urlsplit

DENIED = re.compile(
    r"workbuddy|codebuddy|huaweicloud|huawei[\s-]*cloud|modelarts|codearts|华为云|码道", re.I
)
VALID_ENV = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*$")
VALID_ID = re.compile(r"^[A-Za-z0-9_-]+$")
LOCAL_HOSTS = {"localhost", "127.0.0.1", "::1"}

def transform(registry):
    if not isinstance(registry, list):
        raise ValueError("DSH registry must be a JSON array")
    entries, exclusions, seen = [], [], set()
    for item in registry:
        pid = item.get("id")
        if not isinstance(pid, str) or not VALID_ID.fullmatch(pid) or pid in seen:
            raise ValueError("invalid or duplicate provider id: %r" % (pid,))
        seen.add(pid)
        fmt = item.get("format")
        auth = item.get("auth") or {}
        kind = auth.get("kind")
        base = item.get("baseUrl")
        reason = None
        searchable = " ".join(str(x or "") for x in (
            pid, item.get("name"), item.get("notes"), base
        ))
        if DENIED.search(searchable):
            reason = "upstream excluded"
        elif fmt != "openai-chat":
            reason = "wire adapter not yet implemented"
        elif kind not in ("apiKey", "none"):
            reason = "auth adapter not yet implemented"
        elif kind == "none" and not item.get("local"):
            reason = "remote anonymous provider excluded"
        elif not isinstance(base, str) or not base or any(c in base for c in "<>{}$"):
            reason = "missing/templated base URL"
        else:
            parsed = urlsplit(base)
            if parsed.username or parsed.password or parsed.query or parsed.fragment or not parsed.hostname:
                reason = "unsafe base URL"
            elif kind == "apiKey" and parsed.scheme != "https":
                reason = "API Key provider requires HTTPS"
            elif kind == "none" and not (
                parsed.scheme == "https" or
                (parsed.scheme == "http" and parsed.hostname in LOCAL_HOSTS)
            ):
                reason = "keyless HTTP requires loopback address"
        raw_models = item.get("models") or []
        if not reason and not raw_models:
            reason = "no models"
        if reason:
            exclusions.append({"id": pid, "reason": reason, "format": fmt, "auth_kind": kind})
            continue
        env = str(auth.get("env") or "")
        if kind == "apiKey" and not VALID_ENV.fullmatch(env):
            env = "OFM_KEY_" + pid.upper().replace("-", "_")
        models, used_models = [], set()
        for m in raw_models:
            if not isinstance(m, dict) or not isinstance(m.get("id"), str) or not m["id"].strip():
                continue
            mid = m["id"]
            if mid in used_models:
                continue
            used_models.add(mid)
            context = m.get("contextWindow")
            models.append({
                "id": mid,
                "name": m.get("name") or mid,
                "context_window": context if type(context) is int and context > 0 else 0,
                "vision": m.get("vision") is True,
            })
        if not models:
            exclusions.append({"id": pid, "reason": "invalid model list", "format": fmt, "auth_kind": kind})
            continue
        entries.append({
            "id": pid, "name": item.get("name") or pid, "enabled": False,
            "base_url": base.rstrip("/"), "format": fmt, "auth_kind": kind,
            "key_env": env if kind == "apiKey" else "", "headers": {},
            "models": models,
        })
    return {"providers": entries}, {
        "sourceProviders": len(registry),
        "importedProviders": len(entries),
        "importedModels": sum(len(p["models"]) for p in entries),
        "excluded": exclusions,
        "note": "All imported providers disabled; OAuth/ADC and non-chat wire adapters not implemented.",
    }

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input", type=Path, default=Path("data/dsh-registry.snapshot.json"))
    parser.add_argument("--output", type=Path, default=Path("ofm-providers.json"))
    parser.add_argument("--report", type=Path, default=Path("migration-report.json"))
    args = parser.parse_args()
    source = json.loads(args.input.read_text(encoding="utf-8"))
    output, report = transform(source)
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.report.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(output, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    args.report.write_text(json.dumps(report, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    print(
        "Imported %d disabled providers / %d models; excluded %d providers." %
        (report["importedProviders"], report["importedModels"], len(report["excluded"]))
    )

if __name__ == "__main__":
    main()
