import random
from typing import Dict, Any

class CanvasSpoofer:
    def generate_noise(self) -> Dict[str, Any]:
        return {
            "offset": random.uniform(-1, 1),
            "noise": random.randint(0, 2),
        }