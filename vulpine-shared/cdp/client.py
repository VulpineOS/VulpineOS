from typing import Optional

class CDPClient:
    def __init__(self, host: str, port: int):
        self.host = host
        self.port = port
        self._connection = None
    
    def connect(self) -> bool:
        return True