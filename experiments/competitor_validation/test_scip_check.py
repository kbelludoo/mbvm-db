import copy
from pathlib import Path
import tempfile
import unittest

from scip_check import check_index, key, occurrence_start, utf16_column


class SCIPAdapterTests(unittest.TestCase):
    def test_encoding_and_range_priority(self):
        self.assertEqual(utf16_column(["🧠 Target"], 0, 5, 1), 3)
        self.assertEqual(utf16_column(["🧠 Target"], 0, 3, 2), 3)
        self.assertEqual(utf16_column(["🧠 Target"], 0, 2, 3), 3)
        with self.assertRaises(ValueError):
            utf16_column(["Target"], 0, 0, 0)
        self.assertEqual(occurrence_start({
            "range": [9, 9, 10],
            "TypedRange": {"SingleLineRange": {"line": 1, "start_character": 5}},
        }), (1, 5))

    def test_answers_do_not_depend_on_expected_labels(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "main.go"
            path.write_text("package p\nfunc Target(){}\nfunc Caller(){Target()}\nfunc Value(){_ = Target}\n", encoding="utf-8")
            catalogue = {
                "root": directory,
                "targets": [{"symbol": "p.Target", "uri": path.as_uri(),
                             "position": {"line": 1, "character": 5},
                             "expected_callers": ["p.Caller"]}],
                "call_positions": {key(path, 2, 14): "p.Caller"},
            }
            index = {"documents": [{"relative_path": "main.go", "position_encoding": 1,
                                    "occurrences": [
                                        {"range": [1, 5, 11], "symbol": "target", "symbol_roles": 1},
                                        {"range": [2, 14, 20], "symbol": "target", "symbol_roles": 8},
                                        {"range": [3, 17, 23], "symbol": "target", "symbol_roles": 8},
                                    ]}]}
            before = check_index(catalogue, index)[0]
            changed = copy.deepcopy(catalogue)
            changed["targets"][0]["expected_callers"] = ["p.Unseen"]
            after = check_index(changed, index)[0]
            self.assertEqual(before["actual"], ["p.Caller"])
            self.assertEqual(before["discarded_noncall_test_or_self_ranges"], 1)
            self.assertEqual(before["actual"], after["actual"])
            self.assertTrue(before["exact"])
            self.assertFalse(after["exact"])
            with self.assertRaises(ValueError):
                check_index(catalogue, {"documents": [{"relative_path": "../escape.go"}]})


if __name__ == "__main__":
    unittest.main()
