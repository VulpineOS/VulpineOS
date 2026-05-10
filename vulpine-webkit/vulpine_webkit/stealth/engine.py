import random
from typing import Dict, Any, Optional, Tuple
from vulpine_shared.fingerprint import FingerprintGenerator


class WebKitStealthEngine:
    WEBGL_VENDORS = ["Intel Inc.", "NVIDIA Corporation", "AMD", "Intel Open Source Technology Center"]
    WEBGL_RENDERERS = [
        "Intel Iris OpenGL Engine",
        "Intel UHD Graphics 620",
        "NVIDIA GeForce RTX 3080/PCIe/SSE2",
        "AMD Radeon Pro 5500M",
        "llvmpipe (LLVM 15.0.6, 256 bits)",
    ]
    CANVAS_NOISE_LEVELS = ["low", "medium", "high"]
    SCREEN_RESOLUTIONS = [(1920, 1080), (2560, 1440), (1366, 768), (1440, 900), (3840, 2160)]
    TIMEZONES = ["America/New_York", "America/Chicago", "America/Los_Angeles"]
    LANGUAGES = ["en-US", "en-GB", "de-DE", "fr-FR", "ja-JP"]
    WEBKIT_USER_AGENTS = [
        "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/605.1.15 Safari/605.1.15",
        "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 Safari/605.1.15",
    ]

    def __init__(
        self,
        user_agent: Optional[str] = None,
        platform: Optional[str] = None,
        screen_resolution: Optional[Tuple[int, int]] = None,
        timezone: Optional[str] = None,
        language: Optional[str] = None,
    ):
        self._user_agent = user_agent or random.choice(self.WEBKIT_USER_AGENTS)
        self._platform = platform or "Linux x86_64"
        self._screen_resolution = screen_resolution or random.choice(self.SCREEN_RESOLUTIONS)
        self._timezone = timezone or random.choice(self.TIMEZONES)
        self._language = language or random.choice(self.LANGUAGES)
        self._canvas_noise_seed = random.randint(0, 1000000)
        self._audio_noise_seed = random.randint(0, 1000000)
        self._webgl_noise_seed = random.randint(0, 1000000)
        self._fp_generator = FingerprintGenerator("webkit")

    def _generate_canvas_noise(self) -> Dict[str, Any]:
        noise_type = random.choice(self.CANVAS_NOISE_LEVELS)
        noise_level = {"low": 1, "medium": 2, "high": 3}.get(noise_type, 2)
        return {"noise": noise_level, "offset_range": noise_level}

    def _generate_webgl_spoof(self) -> Dict[str, Any]:
        return {
            "vendor": random.choice(self.WEBGL_VENDORS),
            "renderer": random.choice(self.WEBGL_RENDERERS),
            "noise_seed": self._webgl_noise_seed,
        }

    def _generate_audio_block(self) -> Dict[str, Any]:
        return {"block": True, "noise_seed": self._audio_noise_seed}

    def _generate_screen_spoof(self) -> Dict[str, Any]:
        width, height = self._screen_resolution
        return {
            "width": width,
            "height": height,
            "availWidth": width - random.randint(0, 100),
            "availHeight": height - random.randint(50, 150),
            "colorDepth": random.choice([24, 32]),
            "pixelDepth": random.choice([24, 32]),
        }

    def generate_fingerprint(self) -> Dict[str, Any]:
        fp = self._fp_generator.generate()
        return {
            "user_agent": self._user_agent,
            "browser": "WebKit",
            "platform": self._platform,
            "engine": fp["engine"],
            "canvas": self._generate_canvas_noise(),
            "webgl": self._generate_webgl_spoof(),
            "audio": self._generate_audio_block(),
            "screen": self._generate_screen_spoof(),
            "timezone": self._timezone,
            "language": self._language,
        }

    def apply_stealth(self) -> bool:
        self.generate_fingerprint()
        return True