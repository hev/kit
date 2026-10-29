import json
import tempfile
import unittest
from pathlib import Path
from plan import fold, plan, NAMES


class PlanTests(unittest.TestCase):
    def test_fold_and_rollback(self):
        for kind, target, legacy, value in [("sessions", "outcome_details", "commit_outcome_sha_reverted", "true"), ("sessions", "outcome_details", "ci_workflow_sha_1_conclusion", "failure"), ("evals", "marks", "mark_custom", 4)]:
            with self.subTest(kind=kind, legacy=legacy):
                row = {"id": "r", legacy: value, target: "{}", "text": "untouched"}
                patch, undo = fold(row, kind)
                updated = {**row, **patch}
                key = legacy[5:] if kind == "evals" else legacy
                self.assertEqual(json.loads(updated[target])[key], value)
                self.assertIsNone(updated[legacy])
                self.assertEqual(updated["text"], "untouched")
                self.assertEqual(fold(updated, kind), (None, None))
                self.assertEqual({**updated, **undo}, row)

    def test_conflict_is_not_silently_lost(self):
        with self.assertRaises(ValueError):
            fold({"id": "r", "marks": '{"accuracy":2}', "mark_accuracy": 4}, "evals")

    def test_complete_schema_and_counts(self):
        with tempfile.TemporaryDirectory() as tmp:
            source, output = Path(tmp) / "snapshot", Path(tmp) / "plan"
            source.mkdir()
            for kind, ns in NAMES.items():
                schema = {"session_id": {"type": "string"}, "text": {"type": "string", "full_text_search": True, "embed": {"model": "keep"}}, "unknown": {"type": "int"}}
                if kind == "sessions":
                    schema.update({f"commit_outcome_{i}_reverted": {"type": "string"} for i in range(812)})
                    schema.update({f"ci_workflow_{i}_conclusion": {"type": "string"} for i in range(126)})
                    schema.update({f"static_{i}": {"type": "string", "filterable": i >= 5} for i in range(84)})
                (source / f"{ns}.schema.json").write_text(json.dumps(schema))
                (source / f"{ns}.rows.jsonl").write_text('{"id":"r"}\n')
            plan(source, output)
            for kind, ns in NAMES.items():
                schema = json.loads((output / f"{ns}.schema.json").read_text())
                self.assertFalse(schema["unknown"]["filterable"])
                self.assertFalse(schema["text"]["filterable"])
                self.assertTrue(schema["text"]["full_text_search"])
                self.assertNotIn("embed", schema["text"])
            report = json.loads((output / "report.json").read_text())
            self.assertEqual(report["hev-traces-sessions"]["dynamic_columns"], 938)
            self.assertEqual(report["hev-traces-sessions"]["attributes_before"], 1025)
            self.assertEqual(report["hev-traces-sessions"]["indexed_before"], 1019)
            self.assertEqual(report["hev-traces-sessions"]["indexed_after"], 20)


if __name__ == "__main__":
    unittest.main()
