"""Tests for AppInputs typed accessors."""

import json
import sys
import os

import pytest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from appstore.inputs import AppInputs


class TestStringAccessor:
    def test_existing_key(self):
        inputs = AppInputs({"greeting": "Hello"})
        assert inputs.string("greeting") == "Hello"

    def test_missing_key_default(self):
        inputs = AppInputs({})
        assert inputs.string("greeting", "default") == "default"

    def test_missing_key_empty(self):
        inputs = AppInputs({})
        assert inputs.string("greeting") == ""

    def test_numeric_to_string(self):
        inputs = AppInputs({"port": 8080})
        assert inputs.string("port") == "8080"


class TestIntegerAccessor:
    def test_existing_int(self):
        inputs = AppInputs({"port": 8080})
        assert inputs.integer("port") == 8080

    def test_string_to_int(self):
        inputs = AppInputs({"port": "8080"})
        assert inputs.integer("port") == 8080

    def test_missing_default(self):
        inputs = AppInputs({})
        assert inputs.integer("port", 80) == 80

    def test_missing_zero(self):
        inputs = AppInputs({})
        assert inputs.integer("port") == 0


class TestBooleanAccessor:
    def test_true_bool(self):
        inputs = AppInputs({"enabled": True})
        assert inputs.boolean("enabled") is True

    def test_false_bool(self):
        inputs = AppInputs({"enabled": False})
        assert inputs.boolean("enabled") is False

    def test_true_string(self):
        inputs = AppInputs({"enabled": "true"})
        assert inputs.boolean("enabled") is True

    def test_yes_string(self):
        inputs = AppInputs({"enabled": "yes"})
        assert inputs.boolean("enabled") is True

    def test_one_string(self):
        inputs = AppInputs({"enabled": "1"})
        assert inputs.boolean("enabled") is True

    def test_false_string(self):
        inputs = AppInputs({"enabled": "false"})
        assert inputs.boolean("enabled") is False

    def test_missing_default_false(self):
        inputs = AppInputs({})
        assert inputs.boolean("enabled") is False

    def test_missing_default_true(self):
        inputs = AppInputs({})
        assert inputs.boolean("enabled", True) is True


class TestSecretAccessor:
    def test_secret_returns_value(self):
        inputs = AppInputs({"password": "s3cret"})
        assert inputs.secret("password") == "s3cret"

    def test_secret_missing(self):
        inputs = AppInputs({})
        assert inputs.secret("password") == ""


class TestRaw:
    def test_raw_returns_copy(self):
        data = {"a": "1", "b": "2"}
        inputs = AppInputs(data)
        raw = inputs.raw()
        assert raw == data
        raw["c"] = "3"
        assert "c" not in inputs._data


class TestFromFile:
    def test_load_from_json(self, tmp_path):
        data = {"greeting": "Hello", "port": 8080, "enabled": True}
        path = tmp_path / "inputs.json"
        path.write_text(json.dumps(data))

        inputs = AppInputs.from_file(str(path))
        assert inputs.string("greeting") == "Hello"
        assert inputs.integer("port") == 8080
        assert inputs.boolean("enabled") is True
