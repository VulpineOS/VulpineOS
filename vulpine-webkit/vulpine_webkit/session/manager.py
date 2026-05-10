import uuid
import json
import os
import shutil
import tempfile
from pathlib import Path
from typing import Dict, Any, Optional, List
from datetime import datetime
from threading import Lock


class SessionManager:
    DEFAULT_SESSION_DIR = tempfile.gettempdir()

    def __init__(self, session_dir: Optional[str] = None):
        self._session_dir = session_dir or self.DEFAULT_SESSION_DIR
        self._sessions: Dict[str, Dict[str, Any]] = {}
        self._lock = Lock()
        self._ensure_session_directory()

    def _ensure_session_directory(self) -> None:
        session_path = Path(self._session_dir)
        if not session_path.exists():
            session_path.mkdir(parents=True, exist_ok=True)

    def create_session(self, name: Optional[str] = None) -> Dict[str, Any]:
        session_id = str(uuid.uuid4())
        profile_id = f"profile_{session_id[:8]}"

        user_data_dir = os.path.join(self._session_dir, profile_id)
        os.makedirs(user_data_dir, exist_ok=True)

        session = {
            "session_id": session_id,
            "profile_id": profile_id,
            "user_data_dir": user_data_dir,
            "created_at": datetime.now().isoformat(),
            "cookies": [],
            "local_storage": {},
            "session_storage": {},
            "cache": {},
            "history": [],
            "bookmarks": [],
            "extensions": [],
            "preferences": self._default_preferences(),
        }

        with self._lock:
            self._sessions[session_id] = session

        self._save_session(session)
        return self._serialize_session(session)

    def _default_preferences(self) -> Dict[str, Any]:
        return {
            "homepage": "about:blank",
            "search_engine": "google",
            "download_directory": os.path.expanduser("~/Downloads"),
            "javascript_enabled": True,
            "cookies_enabled": True,
            "popups_enabled": False,
            "images_enabled": True,
            "css_enabled": True,
            "webgl_enabled": True,
            "webrtc_enabled": True,
            "color_scheme": "system",
            "font_size": 14,
            "zoom_level": 1.0,
        }

    def _save_session(self, session: Dict[str, Any]) -> None:
        session_file = Path(session["user_data_dir"]) / "session.json"
        with open(session_file, "w") as f:
            json.dump(session, f, indent=2)

    def get_session(self, session_id: str) -> Optional[Dict[str, Any]]:
        with self._lock:
            session = self._sessions.get(session_id)
            if session:
                return self._serialize_session(session)

        session_file = Path(self._session_dir) / f"profile_{session_id[:8]}" / "session.json"
        if session_file.exists():
            with open(session_file) as f:
                session = json.load(f)
                with self._lock:
                    self._sessions[session_id] = session
                return self._serialize_session(session)
        return None

    def _serialize_session(self, session: Dict[str, Any]) -> Dict[str, Any]:
        return {
            "session_id": session["session_id"],
            "profile_id": session["profile_id"],
            "profile_path": session["user_data_dir"],
            "user_data_dir": session["user_data_dir"],
            "created_at": session["created_at"],
            "cookies": session.get("cookies", []),
        }

    def delete_session(self, session_id: str) -> bool:
        with self._lock:
            session = self._sessions.pop(session_id, None)
            if session:
                user_data_dir = session.get("user_data_dir")
                if user_data_dir and os.path.exists(user_data_dir):
                    try:
                        shutil.rmtree(user_data_dir)
                    except Exception:
                        pass
                return True
        return False

    def list_sessions(self) -> List[Dict[str, Any]]:
        with self._lock:
            return [
                self._serialize_session(s) for s in self._sessions.values()
            ]

    def save_cookies(self, session_id: str, cookies: List[Dict[str, Any]]) -> bool:
        with self._lock:
            session = self._sessions.get(session_id)
            if session:
                session["cookies"] = cookies
                self._save_session(session)
                return True
        return False

    def load_cookies(self, session_id: str) -> List[Dict[str, Any]]:
        with self._lock:
            session = self._sessions.get(session_id)
            if session:
                return session.get("cookies", [])

        session = self.get_session(session_id)
        if session:
            return []
        return []

    def update_local_storage(
        self, session_id: str, key: str, value: Any
    ) -> bool:
        with self._lock:
            session = self._sessions.get(session_id)
            if session:
                session["local_storage"][key] = value
                self._save_session(session)
                return True
        return False

    def get_local_storage(self, session_id: str) -> Dict[str, Any]:
        with self._lock:
            session = self._sessions.get(session_id)
            if session:
                return session.get("local_storage", {})
        return {}

    def update_preferences(
        self, session_id: str, preferences: Dict[str, Any]
    ) -> bool:
        with self._lock:
            session = self._sessions.get(session_id)
            if session:
                session["preferences"].update(preferences)
                self._save_session(session)
                return True
        return False

    def get_preferences(self, session_id: str) -> Dict[str, Any]:
        with self._lock:
            session = self._sessions.get(session_id)
            if session:
                return session.get("preferences", {})
        return {}

    def add_to_history(self, session_id: str, url: str) -> bool:
        with self._lock:
            session = self._sessions.get(session_id)
            if session:
                session["history"].append(
                    {
                        "url": url,
                        "timestamp": datetime.now().isoformat(),
                    }
                )
                self._save_session(session)
                return True
        return False

    def get_history(self, session_id: str) -> List[Dict[str, str]]:
        with self._lock:
            session = self._sessions.get(session_id)
            if session:
                return session.get("history", [])
        return []

    def clone_session(self, source_session_id: str) -> Optional[Dict[str, Any]]:
        source = self.get_session(source_session_id)
        if not source:
            return None

        new_session = self.create_session()
        new_session_id = new_session["session_id"]

        with self._lock:
            source_session = self._sessions.get(source_session_id)
            if source_session:
                new_session_obj = self._sessions.get(new_session_id)
                if new_session_obj:
                    new_session_obj["cookies"] = source_session.get("cookies", []).copy()
                    new_session_obj["local_storage"] = source_session.get(
                        "local_storage", {}
                    ).copy()
                    new_session_obj["preferences"] = source_session.get(
                        "preferences", {}
                    ).copy()
                    self._save_session(new_session_obj)

        return new_session

    def export_session(self, session_id: str, export_path: str) -> bool:
        session = self.get_session(session_id)
        if not session:
            return False

        try:
            session_file = Path(export_path)
            with open(session_file, "w") as f:
                json.dump(session, f, indent=2)
            return True
        except Exception:
            return False

    def import_session(self, import_path: str) -> Optional[Dict[str, Any]]:
        try:
            with open(import_path) as f:
                session_data = json.load(f)

            new_session = self.create_session()
            new_session_id = new_session["session_id"]

            with self._lock:
                new_session_obj = self._sessions.get(new_session_id)
                if new_session_obj:
                    new_session_obj["cookies"] = session_data.get("cookies", [])
                    new_session_obj["local_storage"] = session_data.get(
                        "local_storage", {}
                    )
                    new_session_obj["preferences"] = session_data.get(
                        "preferences", {}
                    )
                    self._save_session(new_session_obj)

            return new_session
        except Exception:
            return None

    def get_session_count(self) -> int:
        with self._lock:
            return len(self._sessions)

    def clear_all_sessions(self) -> int:
        count = 0
        with self._lock:
            session_ids = list(self._sessions.keys())
            for session_id in session_ids:
                if self.delete_session(session_id):
                    count += 1
        return count