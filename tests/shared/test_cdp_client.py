import pytest
from vulpine_shared.cdp import CDPClient

def test_cdp_client_interface():
    client = CDPClient("localhost", 9222)
    assert client.host == "localhost"
    assert client.port == 9222