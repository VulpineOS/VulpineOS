import pytest
from vulpine_webkit.session import SessionManager
import uuid


def test_session_manager_init():
    mgr = SessionManager()
    assert mgr is not None


def test_create_session():
    mgr = SessionManager()
    session = mgr.create_session()
    assert session is not None
    assert "session_id" in session
    assert "profile_id" in session


def test_create_session_returns_unique_ids():
    mgr = SessionManager()
    session1 = mgr.create_session()
    session2 = mgr.create_session()
    assert session1["session_id"] != session2["session_id"]
    assert session1["profile_id"] != session2["profile_id"]


def test_get_session():
    mgr = SessionManager()
    session = mgr.create_session()
    retrieved = mgr.get_session(session["session_id"])
    assert retrieved is not None
    assert retrieved["session_id"] == session["session_id"]


def test_get_nonexistent_session():
    mgr = SessionManager()
    result = mgr.get_session(str(uuid.uuid4()))
    assert result is None


def test_delete_session():
    mgr = SessionManager()
    session = mgr.create_session()
    result = mgr.delete_session(session["session_id"])
    assert result is True
    retrieved = mgr.get_session(session["session_id"])
    assert retrieved is None


def test_list_sessions():
    mgr = SessionManager()
    mgr.create_session()
    mgr.create_session()
    sessions = mgr.list_sessions()
    assert len(sessions) == 2


def test_session_has_profile_path():
    mgr = SessionManager()
    session = mgr.create_session()
    assert "profile_path" in session


def test_session_has_cookies():
    mgr = SessionManager()
    session = mgr.create_session()
    assert "cookies" in session


def test_save_cookies():
    mgr = SessionManager()
    session = mgr.create_session()
    cookies = [{"name": "test", "value": "value"}]
    result = mgr.save_cookies(session["session_id"], cookies)
    assert result is True


def test_load_cookies():
    mgr = SessionManager()
    session = mgr.create_session()
    cookies = [{"name": "test", "value": "value"}]
    mgr.save_cookies(session["session_id"], cookies)
    loaded = mgr.load_cookies(session["session_id"])
    assert loaded == cookies


def test_session_has_created_at():
    mgr = SessionManager()
    session = mgr.create_session()
    assert "created_at" in session


def test_session_has_user_data_dir():
    mgr = SessionManager()
    session = mgr.create_session()
    assert "user_data_dir" in session