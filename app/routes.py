from flask import Blueprint, render_template, request, jsonify, Response
import subprocess
import json
import re
from app.database import (
    create_schedule, get_schedules, get_schedule, 
    update_schedule, delete_schedule, log_execution, get_logs
)

main = Blueprint('main', __name__)

# Storage per progress in tempo reale
progress_data = {}

@main.route('/')
def index():
    return render_template('index.html')

@main.route('/api/rclone', methods=['POST'])
def rclone_command():
    """Esegue comandi rclone con progress tracking"""
    data = request.json
    command = data.get('command', '')
    source = data.get('source', '')
    dest = data.get('dest', '')
    options = data.get('options', '')
    task_id = data.get('task_id')
    
    cmd = ['rclone', '--progress']
    
    if command == 'copy':
        cmd.extend(['copy', source, dest])
    elif command == 'sync':
        cmd.extend(['sync', source, dest])
    elif command == 'move':
        cmd.extend(['move', source, dest])
    elif command == 'delete':
        cmd.extend(['delete', source])
    elif command == 'ls':
        cmd.extend(['ls', source])
    elif command == 'lsl':
        cmd.extend(['lsl', source])
    elif command == 'size':
        cmd.extend(['size', source])
    elif command == 'config':
        cmd.extend(['config', 'list'])
    else:
        return jsonify({'error': 'Comando non valido'}), 400
    
    if options:
        cmd.extend(options.split())
    
    try:
        process = subprocess.Popen(
            cmd,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True,
            bufsize=1
        )
        
        output = []
        progress = {'percent': 0, 'transferred': '0 B', 'speed': '0 B/s', 'eta': 'N/A', 'status': 'running'}
        
        for line in process.stdout:
            output.append(line)
            
            # Parse progress da output rclone
            # Esempio: Transferred:    1.234 GiB / 5.678 GiB, 22%, 56.78 MB/s, ETA 12:34:56
            progress_match = re.search(r'Transferred:\s*([\d.]+\s*\w+)\s*/\s*([\d.]+\s*\w+),\s*([\d.]+)%,\s*([\d.]+\s*\w+/s),\s*ETA\s*(\d+:\d+:\d+)', line)
            if progress_match:
                progress = {
                    'transferred': progress_match.group(1),
                    'total': progress_match.group(2),
                    'percent': float(progress_match.group(3)),
                    'speed': progress_match.group(4),
                    'eta': progress_match.group(5),
                    'status': 'running'
                }
            
            # Check completamento
            if 'Transferred:' in line and 'errors' in line.lower():
                progress['status'] = 'completed_with_errors'
            elif 'Checking' in line or 'Sent' in line:
                progress['status'] = 'running'
        
        process.wait()
        
        return jsonify({
            'stdout': ''.join(output),
            'returncode': process.returncode,
            'progress': progress
        })
    except subprocess.TimeoutExpired:
        process.kill()
        return jsonify({'error': 'Timeout comando'}), 500
    except Exception as e:
        return jsonify({'error': str(e)}), 500

@main.route('/api/progress/<task_id>', methods=['GET'])
def get_progress(task_id):
    """Restituisce progress di un task"""
    if task_id in progress_data:
        return jsonify(progress_data[task_id])
    return jsonify({'error': 'Task non trovato'}), 404

@main.route('/api/remotes', methods=['GET'])
def list_remotes():
    """Lista i remoti configurati"""
    try:
        result = subprocess.run(
            ['rclone', 'listremotes'],
            capture_output=True,
            text=True,
            timeout=30
        )
        remotes = [r.rstrip(':') for r in result.stdout.strip().split('\n') if r]
        return jsonify({'remotes': remotes})
    except Exception as e:
        return jsonify({'error': str(e)}), 500

# === API SCHEDULING ===

@main.route('/api/schedules', methods=['GET'])
def api_get_schedules():
    """Lista tutte le schedulazioni"""
    schedules = get_schedules()
    return jsonify({'schedules': schedules})

@main.route('/api/schedules', methods=['POST'])
def api_create_schedule():
    """Crea una nuova schedulazione"""
    data = request.json
    name = data.get('name', '')
    command = data.get('command', '')
    source = data.get('source', '')
    dest = data.get('dest', '')
    options = data.get('options', '')
    cron_expression = data.get('cron_expression', '0 * * * *')  # Default: ogni ora
    
    if not name or not command or not source:
        return jsonify({'error': 'Campi obbligatori mancanti'}), 400
    
    schedule_id = create_schedule(name, command, source, dest, options, cron_expression)
    return jsonify({'schedule_id': schedule_id, 'message': 'Schedulazione creata'}), 201

@main.route('/api/schedules/<int:schedule_id>', methods=['GET'])
def api_get_schedule(schedule_id):
    """Ottieni dettagli schedulazione"""
    schedule = get_schedule(schedule_id)
    if schedule:
        return jsonify(schedule)
    return jsonify({'error': 'Schedulazione non trovata'}), 404

@main.route('/api/schedules/<int:schedule_id>', methods=['PUT'])
def api_update_schedule(schedule_id):
    """Aggiorna schedulazione"""
    data = request.json
    update_schedule(schedule_id, **data)
    return jsonify({'message': 'Schedulazione aggiornata'})

@main.route('/api/schedules/<int:schedule_id>', methods=['DELETE'])
def api_delete_schedule(schedule_id):
    """Elimina schedulazione"""
    delete_schedule(schedule_id)
    return jsonify({'message': 'Schedulazione eliminata'})

@main.route('/api/logs', methods=['GET'])
def api_get_logs():
    """Ottiene log esecuzioni"""
    limit = request.args.get('limit', 50, type=int)
    logs = get_logs(limit)
    return jsonify({'logs': logs})
