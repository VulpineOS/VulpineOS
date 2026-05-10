from typing import Dict, Any, Optional
import random

class FingerprintGenerator:
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
    
    def __init__(self, browser_type: str):
        if browser_type not in self.BROWSER_CONFIGS:
            raise ValueError(f"Unknown browser: {browser_type}")
        self.browser_type = browser_type
    
    def generate(self) -> Dict[str, Any]:
        config = self.BROWSER_CONFIGS[self.browser_type]
        return {
            "browser": config["browser"],
            "platform": config["platform"],
            "engine": random.choice(config["engines"]),
            "user_agent": self._generate_ua(),
        }
    
    def _generate_ua(self) -> str:
        return "Mozilla/5.0"