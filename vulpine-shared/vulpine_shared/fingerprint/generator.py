from typing import Dict, Any
import random

BROWSER_CONFIGS = {
    "webkit": {
        "browser": "webkit",
        "platform": "Linux",
        "engines": ["WebKit"],
    },
    "otter": {
        "browser": "otter",
        "platform": "Linux",
        "engines": ["WebKit"],
    },
    "palemoon": {
        "browser": "palemoon",
        "platform": "Windows",
        "engines": ["Goanna"],
    },
}

UA_TEMPLATES = {
    "webkit": "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
    "otter": "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Otter/1.1",
    "palemoon": "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Goanna/5.0 Firefox/115.0",
}

class FingerprintGenerator:
    def __init__(self, browser_type: str, seed: int | None = None):
        if browser_type not in BROWSER_CONFIGS:
            raise ValueError(f"Unknown browser: {browser_type}")
        self.browser_type = browser_type
        self._random = random.Random(seed)

    def generate(self) -> Dict[str, Any]:
        config = BROWSER_CONFIGS[self.browser_type]
        ua_template = UA_TEMPLATES[self.browser_type]
        result = {
            "browser": config["browser"],
            "platform": config["platform"],
            "engine": self._random.choice(config["engines"]),
            "user_agent": ua_template,
        }
        self._validate_output(result)
        return result

    def _validate_output(self, data: Dict[str, Any]) -> None:
        required_keys = {"browser", "platform", "engine", "user_agent"}
        if not required_keys.issubset(data.keys()):
            raise ValueError(f"Missing required keys: {required_keys - data.keys()}")
        for key in required_keys:
            if not isinstance(data[key], str):
                raise ValueError(f"Key '{key}' must be str, got {type(data[key]).__name__}")