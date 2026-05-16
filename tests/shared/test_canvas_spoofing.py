import pytest
from vulpine_shared.stealth.canvas import CanvasSpoofer

def test_canvas_noise_injection():
    spoofer = CanvasSpoofer()
    noise = spoofer.generate_noise()
    assert len(noise) > 0
    assert isinstance(noise, dict)