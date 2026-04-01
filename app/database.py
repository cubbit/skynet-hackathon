import sqlite3
from datetime import datetime
import os

DB_PATH = os.path.join(os.path.dirname(__file__), '..', 'rclone.db')

def get_db():
    conn = sqlite3.connect(DB_PATH)
    conn.row_factory = sqlite3.Row
    return conn

def init_db():
    conn = get_db()
    cursor = conn.cursor()
    
    # Tabella schedulazioni
    cursor.execute('''
        CREATE TABLE IF NOT EXISTS schedules (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            name TEXT NOT NULL,
            command TEXT NOT NULL,
            source TEXT NOT NULL,
            dest TEXT,
            options TEXT,
            cron_expression TEXT NOT NULL,
            enabled BOOLEAN DEFAULT 1,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        )
    ''')
    
    # Tabella log esecuzioni
    cursor.execute('''
        CREATE TABLE IF NOT EXISTS execution_logs (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            schedule_id INTEGER,
            command TEXT NOT NULL,
            source TEXT,
            dest TEXT,
            status TEXT NOT NULL,
            output TEXT,
            started_at TIMESTAMP,
            completed_at TIMESTAMP,
            FOREIGN KEY (schedule_id) REFERENCES schedules(id)
        )
    ''')
    
    conn.commit()
    conn.close()

def create_schedule(name, command, source, dest, options, cron_expression):
    conn = get_db()
    cursor = conn.cursor()
    cursor.execute('''
        INSERT INTO schedules (name, command, source, dest, options, cron_expression)
        VALUES (?, ?, ?, ?, ?, ?)
    ''', (name, command, source, dest, options, cron_expression))
    conn.commit()
    schedule_id = cursor.lastrowid
    conn.close()
    return schedule_id

def get_schedules():
    conn = get_db()
    cursor = conn.cursor()
    cursor.execute('SELECT * FROM schedules ORDER BY created_at DESC')
    schedules = [dict(row) for row in cursor.fetchall()]
    conn.close()
    return schedules

def get_schedule(schedule_id):
    conn = get_db()
    cursor = conn.cursor()
    cursor.execute('SELECT * FROM schedules WHERE id = ?', (schedule_id,))
    row = cursor.fetchone()
    conn.close()
    return dict(row) if row else None

def update_schedule(schedule_id, **kwargs):
    conn = get_db()
    cursor = conn.cursor()
    
    fields = []
    values = []
    for key, value in kwargs.items():
        if key != 'id':
            fields.append(f'{key} = ?')
            values.append(value)
    
    if fields:
        values.append(schedule_id)
        query = f'UPDATE schedules SET {", ".join(fields)}, updated_at = CURRENT_TIMESTAMP WHERE id = ?'
        cursor.execute(query, values)
        conn.commit()
    
    conn.close()

def delete_schedule(schedule_id):
    conn = get_db()
    cursor = conn.cursor()
    cursor.execute('DELETE FROM schedules WHERE id = ?', (schedule_id,))
    conn.commit()
    conn.close()

def log_execution(schedule_id, command, source, dest, status, output=None):
    conn = get_db()
    cursor = conn.cursor()
    cursor.execute('''
        INSERT INTO execution_logs (schedule_id, command, source, dest, status, output, started_at, completed_at)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?)
    ''', (schedule_id, command, source, dest, status, output, datetime.now(), datetime.now()))
    conn.commit()
    conn.close()

def get_logs(limit=50):
    conn = get_db()
    cursor = conn.cursor()
    cursor.execute('''
        SELECT e.*, s.name as schedule_name 
        FROM execution_logs e 
        LEFT JOIN schedules s ON e.schedule_id = s.id 
        ORDER BY e.started_at DESC 
        LIMIT ?
    ''', (limit,))
    logs = [dict(row) for row in cursor.fetchall()]
    conn.close()
    return logs

# Inizializza DB all'import
init_db()
