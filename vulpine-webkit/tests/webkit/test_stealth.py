import pytest
from vulpine_shared.fingerprint import FingerprintGenerator


class TestWebKitStealthEngine:
    def test_init(self):
        from vulpine_webkit.stealth import WebKitStealthEngine
        engine = WebKitStealthEngine()
        assert engine is not None

    def test_generate_fingerprint_returns_expected_keys(self):
        from vulpine_webkit.stealth import WebKitStealthEngine
        engine = WebKitStealthEngine()
        fp = engine.generate_fingerprint()
        expected = ["user_agent", "browser", "platform", "engine", "canvas", "webgl", "audio", "screen", "timezone", "language"]
        for key in expected:
            assert key in fp, f"Missing key: {key}"

    def test_fingerprint_generator_for_webkit(self):
        from vulpine_webkit.stealth import WebKitStealthEngine
        engine = WebKitStealthEngine()
        assert hasattr(engine, "_fp_generator")
        assert engine._fp_generator.browser_type == "webkit"

    def test_canvas_noise_in_fingerprint(self):
        from vulpine_webkit.stealth import WebKitStealthEngine
        engine = WebKitStealthEngine()
        fp = engine.generate_fingerprint()
        canvas = fp["canvas"]
        assert "noise" in canvas
        assert "offset_range" in canvas

    def test_webgl_spoof_in_fingerprint(self):
        from vulpine_webkit.stealth import WebKitStealthEngine
        engine = WebKitStealthEngine()
        fp = engine.generate_fingerprint()
        webgl = fp["webgl"]
        assert "vendor" in webgl
        assert "renderer" in webgl

    def test_audio_blocking_in_fingerprint(self):
        from vulpine_webkit.stealth import WebKitStealthEngine
        engine = WebKitStealthEngine()
        fp = engine.generate_fingerprint()
        audio = fp["audio"]
        assert "block" in audio
        assert audio["block"] is True

    def test_apply_stealth_returns_bool(self):
        from vulpine_webkit.stealth import WebKitStealthEngine
        engine = WebKitStealthEngine()
        result = engine.apply_stealth()
        assert isinstance(result, bool)

    def test_custom_options(self):
        from vulpine_webkit.stealth import WebKitStealthEngine
        engine = WebKitStealthEngine(
            user_agent="Custom/1.0",
            platform="Linux x86_64",
            screen_resolution=(1920, 1080),
        )
        fp = engine.generate_fingerprint()
        assert fp["user_agent"] == "Custom/1.0"
        assert fp["platform"] == "Linux x86_64"
        assert fp["screen"]["width"] == 1920
        assert fp["screen"]["height"] == 1080