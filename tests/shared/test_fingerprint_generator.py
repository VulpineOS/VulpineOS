import pytest
from vulpine_shared.fingerprint import FingerprintGenerator

def test_generate_webkit_fingerprint():
    gen = FingerprintGenerator("webkit")
    fp = gen.generate()
    assert "user_agent" in fp
    assert "platform" in fp
    assert fp["browser"] == "webkit"

def test_generate_palemoon_fingerprint():
    gen = FingerprintGenerator("palemoon")
    fp = gen.generate()
    assert fp["browser"] == "palemoon"
    assert "goanna" in fp["engine"].lower()