import importlib.util
import json
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location("dsh_import", ROOT / "tools" / "import_dsh_registry.py")
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)

class TestDSHImport(unittest.TestCase):
    def test_chat_apikey_and_local_none(self):
        rows = [
          {"id":"remote","name":"Remote","format":"openai-chat","auth":{"kind":"apiKey","env":"REMOTE_KEY"},"baseUrl":"https://example.org/v1","models":[{"id":"org/model","vision":True}]},
          {"id":"local","name":"Local","format":"openai-chat","auth":{"kind":"none"},"local":True,"baseUrl":"http://127.0.0.1:11434/v1","models":[{"id":"model"}]},
          {"id":"oauth","format":"openai-chat","auth":{"kind":"oauth"},"baseUrl":"https://api.example.org/v1","models":[{"id":"a"}]},
          {"id":"messages","format":"anthropic-messages","auth":{"kind":"apiKey"},"baseUrl":"https://api.example.org/v1","models":[{"id":"a"}]},
          {"id":"anonymous","format":"openai-chat","auth":{"kind":"none"},"baseUrl":"https://example.net/v1","models":[{"id":"a"}]},
        ]
        output, manifest = module.transform(rows)
        self.assertEqual(manifest["importedProviders"], 2)
        self.assertEqual(len(manifest["excluded"]), 3)
        self.assertTrue(all(p["enabled"] is False for p in output["providers"]))
        self.assertEqual(output["providers"][0]["models"][0]["id"], "org/model")
        self.assertEqual(output["providers"][1]["auth_kind"], "none")
        self.assertNotIn('"api_key":', json.dumps(output))

    def test_duplicate_provider_is_rejected(self):
        with self.assertRaises(ValueError):
            module.transform([{"id":"duplicated"},{"id":"duplicated"}])

    def test_real_pinned_catalog_is_complete_and_disabled(self):
        rows = json.loads((ROOT / "data" / "dsh-registry.snapshot.json").read_text(encoding="utf-8"))
        self.assertEqual(len(rows), 76)
        self.assertEqual(sum(len(row["models"]) for row in rows), 507)
        output, report = module.transform(rows)
        self.assertGreater(report["importedProviders"], 50)
        self.assertEqual(len(output["providers"]), report["importedProviders"])
        self.assertTrue(all(row["enabled"] is False for row in output["providers"]))
        self.assertFalse(any(row["auth_kind"] in ("oauth","adc") for row in output["providers"]))
        self.assertFalse(any(row["format"] != "openai-chat" for row in output["providers"]))

    def test_no_import_on_unsafe_urls(self):
        rows = [{"id":"a","format":"openai-chat","auth":{"kind":"apiKey","env":"A_KEY"},"baseUrl":"https://user:secret@example.org/v1","models":[{"id":"x"}]}]
        output, report = module.transform(rows)
        self.assertEqual(output["providers"], [])
        self.assertEqual(report["excluded"][0]["reason"], "unsafe base URL")

if __name__ == "__main__":
    unittest.main()
