#!/usr/bin/env python3
"""Permanent binary checks: recoverable delete, move, restore and replacement;
consent, independent review, linked decisions, pending notifications, recording
and transfer failures, unreused action IDs, CLI independence,
built-in trash and retention settings, values that replace unusable ones, review patterns, folder requests, malformed
patterns, and the JSON form of results.

Uses copies of a supplied real document. All effects stay in the supplied test
directory. The caller retains that directory for inspection and cleanup.
"""
from datetime import datetime, timedelta, timezone
import json
import os
from pathlib import Path
import pty
import shutil
import socket
import sqlite3
import subprocess
import sys
import tomllib


class Commands:
    def __init__(self, binary, directory, document):
        self.binary = str(Path(binary).resolve(strict=True))
        self.directory = Path(directory).resolve()
        self.directory.mkdir(parents=True, exist_ok=False)
        self.document = Path(document).resolve(strict=True)
        self.environment = dict(os.environ, HOME=str(self.directory), USERPROFILE=str(self.directory))
        catalog_path = Path(__file__).resolve().parents[1] / 'internal' / 'appstrings' / 'app-strings.toml'
        self.catalog = tomllib.loads(catalog_path.read_text())

    def invoke(self, *arguments, answer='', exit_code=0):
        process = subprocess.run([self.binary, arguments[0], '--json', *arguments[1:]],
                                 env=self.environment, input=answer, capture_output=True, text=True, timeout=10)
        assert process.returncode == exit_code, (arguments, process.returncode, process.stdout, process.stderr)
        return process

    def run(self, *arguments, answer='', exit_code=0):
        return self.printed_json(self.invoke(*arguments, answer=answer, exit_code=exit_code))

    def printed_json(self, process):
        """Return the JSON that a command wrote to standard output. A delete, move, dispose, or
        restore result that holds one action and nothing else is returned as that action."""
        parsed_output = json.loads(process.stdout)
        holds_one_action = (isinstance(parsed_output, dict) and list(parsed_output) == ['actions']
                            and len(parsed_output['actions']) == 1)
        if process.args[1] in ('delete', 'move', 'dispose', 'restore') and holds_one_action:
            return parsed_output['actions'][0]
        return parsed_output

    def source(self, name):
        source = self.directory / name
        shutil.copyfile(self.document, source)
        return source

    def configure(self, review):
        settings = self.directory / '.saferm'
        settings.mkdir(exist_ok=True)
        (settings / 'config.toml').write_text(
            'trash-path = ' + json.dumps(str(self.directory / 'trash')) + '\n'
            'retain-period = 30\nrequire-review = ' + json.dumps(review) + '\n')


def baseline(commands):
    commands.configure([])
    source = commands.source('document')
    deleted = commands.run('delete', str(source), 'Remove obsolete document')
    assert deleted['status'] == 1 and not source.exists()
    restored = commands.run('restore', str(deleted['id']))
    assert restored['status'] == 4 and source.read_bytes() == commands.document.read_bytes()
    commands.configure([str(source)])
    pending = commands.run('delete', str(source), 'Request document removal')
    assert pending['status'] == 2 and source.exists()
    denied = commands.run('dispose', str(pending['id']), 'deny')
    assert denied['status'] == 3 and source.read_bytes() == commands.document.read_bytes()
    pending = commands.run('delete', str(source), 'Request document removal')
    approved = commands.run('dispose', str(pending['id']), 'approve')
    assert approved['status'] == 1 and not source.exists()
    commands.run('restore', str(pending['id']))
    commands.configure([])
    destination = commands.directory / 'moved-document'
    moved = commands.run('move', str(source), str(destination), 'Relocate document')
    assert moved['status'] == 1 and destination.read_bytes() == commands.document.read_bytes()
    commands.run('restore', str(moved['id']))
    assert source.read_bytes() == commands.document.read_bytes() and not destination.exists()
    print('PASS: delete, move, restore, rule-based approval and denial without SafeRM Notify')


def completed_record(commands):
    # Permanent: ordinary history starts with the completed outcome.
    commands.configure([])
    commands.run('list')
    database_path = commands.directory / '.saferm' / 'saferm.sqlite'
    with sqlite3.connect(database_path) as database:
        database.executescript('''
            CREATE TABLE initial_outcomes (status INTEGER, resolved_at TEXT, in_progress INTEGER);
            CREATE TRIGGER capture_outcome AFTER INSERT ON actions BEGIN
                INSERT INTO initial_outcomes VALUES (NEW.status, NEW.resolved_at, NEW.in_progress);
            END;
        ''')
    for operation in ('delete', 'move'):
        source = commands.source(operation + '-document')
        arguments = [operation, str(source)]
        if operation == 'move':
            arguments.append(str(commands.directory / 'destination-document'))
        action = commands.run(*arguments, 'Remove obsolete document')
        assert action['status'] == 1 and not source.exists(), action
    with sqlite3.connect(database_path) as database:
        outcomes = database.execute('SELECT status, resolved_at, in_progress FROM initial_outcomes').fetchall()
    assert len(outcomes) == 2 and all(status == 1 and resolved and not busy for status, resolved, busy in outcomes), outcomes
    print('PASS: ordinary history is created with the completed outcome')


def occupied_move_refusal(commands):
    # Kind: permanent. An ordinary move preserves both occupied paths and adds no history.
    commands.configure([])
    source = commands.source('refused-move-source')
    destination = commands.source('refused-move-destination')
    occupant_bytes = (Path(__file__).resolve().parents[1] / 'LICENSE').read_bytes()
    destination.write_bytes(occupant_bytes)
    previous_history = commands.run('list')
    process = commands.invoke('move', str(source), str(destination), 'Relocate document', exit_code=1)
    assert not process.stdout and commands.catalog['error']['fileExists'] in process.stderr, process
    assert source.read_bytes() == commands.document.read_bytes()
    assert destination.read_bytes() == occupant_bytes
    assert commands.run('list') == previous_history
    print('PASS: an ordinary move refuses an occupied destination without changes')


def execution_before_recording(commands):
    # Permanent: a rejected history insertion happens after filesystem execution.
    database_path = commands.directory / '.saferm' / 'saferm.sqlite'
    with sqlite3.connect(database_path) as database:
        recorded_count = database.execute('SELECT COUNT(*) FROM actions').fetchone()[0]
        database.executescript('''
            CREATE TRIGGER reject_record BEFORE INSERT ON actions BEGIN
                SELECT RAISE(FAIL, 'verification rejects recording');
            END;
        ''')
    for operation in ('delete', 'move'):
        source = commands.source(operation + '-unrecorded-document')
        arguments = [operation, '--json', str(source)]
        destination = commands.directory / 'unrecorded-destination-document'
        if operation == 'move':
            arguments.append(str(destination))
        previous_trash = set((commands.directory / 'trash').iterdir())
        process = subprocess.run([commands.binary, *arguments, 'Remove obsolete document'],
                                 env=commands.environment, capture_output=True, text=True, timeout=10)
        assert process.returncode != 0, process.stdout
        assert not source.exists(), (operation, process.stdout, process.stderr)
        if operation == 'move':
            assert destination.read_bytes() == commands.document.read_bytes()
        else:
            added_trash = set((commands.directory / 'trash').iterdir()) - previous_trash
            assert len(added_trash) == 1
            assert added_trash.pop().read_bytes() == commands.document.read_bytes()
    with sqlite3.connect(database_path) as database:
        assert database.execute('SELECT COUNT(*) FROM actions').fetchone()[0] == recorded_count
    print('PASS: delete and move execute before a rejected history insertion')


def request_push(commands):
    # Permanent: a saved approval request pushes its ID; SafeRM Notify is not required for decisions.
    commands.configure([])
    source = commands.source('review-document')
    commands.configure([str(source)])
    with socket.socket() as listener:
        listener.bind(('127.0.0.1', 0))
        listener.listen()
        listener.settimeout(1)
        address = '127.0.0.1:' + str(listener.getsockname()[1])
        endpoint = commands.directory / '.saferm' / 'notifications.json'
        endpoint.write_text(json.dumps({'address': address, 'token': 'test-token'}))
        pending = commands.run('delete', str(source), 'Request document removal')
        connection, _ = listener.accept()
        with connection:
            message = json.load(connection.makefile())
        assert message == {'id': pending['id'], 'token': 'test-token'}, message
        assert commands.run('view', str(message['id']))['status'] == 2 and source.exists()
    denied = commands.run('dispose', str(pending['id']), 'deny')
    assert denied['status'] == 3 and source.exists()
    print('PASS: request pushed after saving; manual denial works without SafeRM Notify')


def unreused_ids(commands):
    # Permanent: an ID removed by cleanup is never issued again.
    commands.configure([])
    kept = commands.run('delete', str(commands.source('kept-document')), 'Remove obsolete document')
    destination = commands.directory / 'orphaned-destination-document'
    orphaned = commands.run('move', str(commands.source('orphaned-document')), str(destination), 'Relocate document')
    assert orphaned['id'] > kept['id'], (kept, orphaned)
    destination.rename(commands.directory / 'relocated-document')
    cleaned = commands.run('cleanup', '--orphans')
    assert cleaned['removed'] == [orphaned['id']], cleaned
    later = commands.run('delete', str(commands.source('later-document')), 'Remove obsolete document')
    assert later['id'] > orphaned['id'], (orphaned, later)
    print('PASS: an ID removed by cleanup is never issued again')


def restore_preservation(commands):
    # Kind: permanent. Confirmed restore records and retains the occupying item.
    for original_type in ('delete', 'move'):
        commands.configure([])
        source = commands.source(original_type + '-restore-source')
        arguments = [original_type, str(source)]
        if original_type == 'move':
            arguments.append(str(commands.directory / 'restore-moved-document'))
        original = commands.run(*arguments, 'Retain original document')
        occupant_bytes = (Path(__file__).resolve().parents[1] / 'LICENSE').read_bytes()
        source.write_bytes(occupant_bytes)
        before_confirmation = commands.run('list')
        force_only = commands.invoke('restore', '--force', str(original['id']), exit_code=1)
        assert not force_only.stdout and commands.catalog['error']['fileExists'] in force_only.stderr
        assert commands.run('list') == before_confirmation and source.read_bytes() == occupant_bytes
        for answer in ('n\n', '', 'yes'):
            refused = commands.invoke('restore', '--overwrite', str(original['id']), answer=answer)
            assert json.loads(refused.stdout)['notice'] == commands.catalog['notice']['canceled'], refused
            assert json.loads(refused.stderr)['path'] == original['source'], refused.stderr
            assert commands.run('list') == before_confirmation
            assert source.read_bytes() == occupant_bytes
        restored = commands.run('restore', '--overwrite', str(original['id']), answer='y\n')
        assert len(restored['actions']) == 2, restored
        saved_occupant, restored_original = restored['actions']
        assert saved_occupant['type'] == 'delete' and saved_occupant['status'] == 1
        assert str(original['id']) in saved_occupant['reason']
        assert restored_original['id'] == original['id'] and restored_original['status'] == 4
        assert source.read_bytes() == commands.document.read_bytes()
        trash_item = commands.directory / 'trash' / saved_occupant['trashID']
        assert trash_item.read_bytes() == occupant_bytes
        assert commands.run('view', str(saved_occupant['id'])) == saved_occupant
    print('PASS: confirmed restore preserves the occupant in recorded recoverable trash')


def restore_review(commands):
    # Kind: permanent. Force cannot bypass occupant review; the restore remains available.
    commands.configure([])
    source = commands.source('protected-restore-source')
    original = commands.run('delete', str(source), 'Retain original document')
    occupant_bytes = (Path(__file__).resolve().parents[1] / 'LICENSE').read_bytes()
    source.write_bytes(occupant_bytes)
    commands.configure([str(source)])
    pending_process = commands.invoke('restore', '--overwrite', '--force', str(original['id']), exit_code=1)
    pending_delete = commands.printed_json(pending_process)
    assert pending_delete['type'] == 'delete' and pending_delete['status'] == 2
    error_text = commands.catalog['error']['restorePendingDetail'].replace('%w', commands.catalog['error']['restorePending'])
    assert json.loads(pending_process.stderr)['error'] == error_text % pending_delete['id'], pending_process.stderr
    assert commands.run('view', str(original['id'])) == original
    assert source.read_bytes() == occupant_bytes
    commands.run('dispose', str(pending_delete['id']), 'approve')
    restored = commands.run('restore', str(original['id']))
    assert restored['status'] == 4 and source.read_bytes() == commands.document.read_bytes()
    assert (commands.directory / 'trash' / pending_delete['trashID']).read_bytes() == occupant_bytes
    print('PASS: occupant review survives force and leaves the original restore available')


def restore_recording_failures(commands):
    # Kind: permanent. Recording failures retain partial output and each item's recovery location.
    commands.configure([])
    for failure_at in ('occupant', 'claim', 'outcome'):
        source = commands.source('restore-failure-' + failure_at)
        original = commands.run('delete', str(source), 'Retain original document')
        occupant_bytes = (Path(__file__).resolve().parents[1] / 'LICENSE').read_bytes()
        source.write_bytes(occupant_bytes)
        statement = {
            'occupant': 'BEFORE INSERT ON actions',
            'claim': 'BEFORE UPDATE ON actions WHEN NEW.in_progress=1',
            'outcome': 'BEFORE UPDATE ON actions WHEN NEW.status=4',
        }[failure_at]
        database_path = commands.directory / '.saferm' / 'saferm.sqlite'
        with sqlite3.connect(database_path) as database:
            database.execute('CREATE TRIGGER reject_restore ' + statement +
                             " BEGIN SELECT RAISE(FAIL,'recording rejected by verification'); END")
        try:
            process = commands.invoke('restore', '--overwrite', '--force', str(original['id']), exit_code=1)
            response = json.loads(process.stdout)
            affected_actions = response['actions'] if 'actions' in response else [response]
            saved_occupant = affected_actions[0]
            assert saved_occupant['type'] == 'delete' and saved_occupant['status'] == 1
            assert saved_occupant['id'] != original['id']
            assert (commands.directory / 'trash' / saved_occupant['trashID']).read_bytes() == occupant_bytes
            assert commands.catalog['error']['databaseWrite'] in process.stderr, process.stderr
            if failure_at == 'outcome':
                assert len(affected_actions) == 2 and affected_actions[1]['id'] == original['id']
                assert source.read_bytes() == commands.document.read_bytes()
            else:
                assert not source.exists()
                assert commands.run('view', str(original['id'])) == original
                assert (commands.directory / 'trash' / original['trashID']).read_bytes() == commands.document.read_bytes()
            if failure_at == 'occupant':
                assert saved_occupant['id'] == 0 and commands.catalog['error']['databaseResult'] in process.stderr
            else:
                assert commands.run('view', str(saved_occupant['id'])) == saved_occupant
        finally:
            with sqlite3.connect(database_path) as database:
                database.execute('DROP TRIGGER reject_restore')
    print('PASS: restore recording errors expose partial results and retained contents')


def move_replacement(commands, listener):
    # Kind: permanent. Replacement preserves both documents, metadata, and ordered linked records.
    commands.configure([])
    source = commands.source('replacement-source')
    destination = commands.directory / 'replacement-destination'
    occupant_bytes = (Path(__file__).resolve().parents[1] / 'LICENSE').read_bytes()
    destination.write_bytes(occupant_bytes)
    previous_history = commands.run('list')
    for flags in ([], ['--force']):
        refused = commands.invoke('move', *flags, str(source), str(destination), 'Relocate document', exit_code=1)
        assert commands.catalog['error']['fileExists'] in refused.stderr
        assert not refused.stdout and commands.run('list') == previous_history
    for answer in ('n\n', '', 'yes'):
        refused = commands.invoke('move', '--overwrite', str(source), str(destination), 'Relocate document', answer=answer)
        assert json.loads(refused.stdout)['notice'] == commands.catalog['notice']['canceled']
        assert json.loads(refused.stderr)['path'] == str(destination)
        assert commands.run('list') == previous_history
        assert source.read_bytes() == commands.document.read_bytes() and destination.read_bytes() == occupant_bytes
    replaced = commands.run('move', '--overwrite', '--requester', 'Agent', '--description', 'Document relocation',
                            str(source), str(destination), 'Relocate document', answer='y\n')
    assert len(replaced['actions']) == 2, replaced
    moved, deleted = replaced['actions']
    assert moved['type'] == 'move' and deleted['type'] == 'delete'
    assert 0 < moved['id'] < deleted['id'] and moved['blockerID'] == deleted['id']
    assert moved['status'] == deleted['status'] == 1
    assert all(action['reason'] == 'Relocate document' and action['requester'] == 'Agent'
               and action['description'] == 'Document relocation' for action in replaced['actions'])
    assert not source.exists() and destination.read_bytes() == commands.document.read_bytes()
    assert (commands.directory / 'trash' / deleted['trashID']).read_bytes() == occupant_bytes
    assert commands.run('view', str(moved['id'])) == moved and commands.run('view', str(deleted['id'])) == deleted
    assert_notifications(listener, [])
    print('PASS: replacement consent, force scope, ordered history, metadata, and recoverable contents')


def move_review(commands, listener):
    # Kind: permanent. Source and occupant reviews independently control execution and notifications.
    for source_review, occupant_review in ((False, True), (True, False), (True, True)):
        name = f'review-{source_review}-{occupant_review}'
        source = commands.source(name + '-source')
        destination = commands.directory / (name + '-destination')
        occupant_bytes = (Path(__file__).resolve().parents[1] / 'LICENSE').read_bytes()
        destination.write_bytes(occupant_bytes)
        rules = ([str(source)] if source_review else []) + ([str(destination)] if occupant_review else [])
        commands.configure(rules)
        replaced = commands.run('move', '--overwrite', '-f', str(source), str(destination), 'Relocate document')
        moved, deleted = replaced['actions']
        assert 0 < moved['id'] < deleted['id'] and moved['blockerID'] == deleted['id']
        assert moved['status'] == 2 and bool(moved.get('reviewRule')) == source_review
        assert deleted['status'] == (2 if occupant_review else 1)
        assert bool(deleted.get('reviewRule')) == occupant_review
        assert source.read_bytes() == commands.document.read_bytes()
        assert_notifications(listener, ([deleted['id']] if occupant_review else []) + [moved['id']])
        if occupant_review:
            assert destination.read_bytes() == occupant_bytes
            commands.run('dispose', str(deleted['id']), 'approve')
        current_move = commands.run('view', str(moved['id']))
        assert current_move['status'] == (2 if source_review else 1)
        if source_review:
            assert not destination.exists() and source.read_bytes() == commands.document.read_bytes()
            commands.run('dispose', str(moved['id']), 'approve')
        assert not source.exists() and destination.read_bytes() == commands.document.read_bytes()
        assert (commands.directory / 'trash' / deleted['trashID']).read_bytes() == occupant_bytes
        assert_notifications(listener, [])
    print('PASS: independent source and occupant review, direct approval, and pending-only notifications')


def move_linked_decisions(commands, listener):
    # Kind: permanent. Combined decisions wait for consent naming both actions and preserve all bytes.
    for source_review in (False, True):
        for decision in ('approve', 'deny'):
            name = f'combined-{source_review}-{decision}'
            source = commands.source(name + '-source')
            destination = commands.directory / (name + '-destination')
            occupant_bytes = (Path(__file__).resolve().parents[1] / 'LICENSE').read_bytes()
            destination.write_bytes(occupant_bytes)
            commands.configure([str(destination)] + ([str(source)] if source_review else []))
            replaced = commands.run('move', '--overwrite', '--force', str(source), str(destination), 'Relocate document')
            moved, deleted = replaced['actions']
            assert_notifications(listener, [deleted['id'], moved['id']])
            previous_history = commands.run('list')
            for answer in ('n\n', ''):
                refused = commands.invoke('dispose', str(moved['id']), decision, answer=answer)
                question = json.loads(refused.stderr)
                assert question['moveID'] == moved['id'] and question['blockerID'] == deleted['id']
                assert question['path'] == str(destination)
                assert str(moved['id']) in question['prompt'] and str(deleted['id']) in question['prompt']
                assert str(destination) in question['prompt']
                assert json.loads(refused.stdout)['notice'] == commands.catalog['notice']['canceled']
                assert commands.run('list') == previous_history
                assert source.read_bytes() == commands.document.read_bytes() and destination.read_bytes() == occupant_bytes
            decided = commands.run('dispose', str(moved['id']), decision, answer='yes\n')
            assert len(decided['actions']) == 2 and {action['id'] for action in decided['actions']} == {moved['id'], deleted['id']}
            status = 1 if decision == 'approve' else 3
            for action in decided['actions']:
                assert action['status'] == status and action['decision'] == decision and action['resolvedAt']
                assert commands.run('view', str(action['id'])) == action
            if decision == 'approve':
                assert not source.exists() and destination.read_bytes() == commands.document.read_bytes()
                assert (commands.directory / 'trash' / deleted['trashID']).read_bytes() == occupant_bytes
            else:
                assert source.read_bytes() == commands.document.read_bytes() and destination.read_bytes() == occupant_bytes
                denied_move = commands.run('view', str(moved['id']))
                assert str(deleted['id']) in denied_move['failure']
            assert_notifications(listener, [])
    print('PASS: combined approval and denial require explicit consent for both actions')


def move_delete_denial(commands, listener):
    # Kind: permanent. A refused delete approval remains deniable and its denial closes the move.
    for missing_occupant in (False, True):
        source = commands.source(f'denied-{missing_occupant}-source')
        destination = commands.source(f'denied-{missing_occupant}-destination')
        commands.configure([str(destination)])
        replaced = commands.run('move', '--overwrite', '--force', str(source), str(destination), 'Relocate document')
        moved, deleted = replaced['actions']
        assert_notifications(listener, [deleted['id'], moved['id']])
        if missing_occupant:
            destination.rename(destination.with_suffix('.retained'))
            previous_history = commands.run('list')
            refused = commands.invoke('dispose', str(moved['id']), 'approve', answer='y\n', exit_code=1)
            assert commands.catalog['error']['fileInspect'] in refused.stderr
            assert commands.run('list') == previous_history
        denied = commands.run('dispose', str(deleted['id']), 'deny')
        assert len(denied['actions']) == 2
        assert all(action['status'] == 3 and action['decision'] == 'deny' and action['resolvedAt'] for action in denied['actions'])
        denied_move = commands.run('view', str(moved['id']))
        assert str(deleted['id']) in denied_move['failure']
        assert source.read_bytes() == commands.document.read_bytes()
        retained_destination = destination.with_suffix('.retained') if missing_occupant else destination
        assert retained_destination.read_bytes() == commands.document.read_bytes()
        assert_notifications(listener, [])
    print('PASS: direct delete denial closes its move, including after a refused approval')


def move_transfer_failure(commands, listener):
    # Kind: permanent. Permission-denied trash creation records a failed delete and denies its move.
    commands.configure([])
    source = commands.source('failed-transfer-source')
    destination = commands.source('failed-transfer-destination')
    parent = commands.directory / 'restricted-trash-parent'
    parent.mkdir()
    trash = parent / 'trash'
    (commands.directory / '.saferm' / 'config.toml').write_text('trash-path = ' + json.dumps(str(trash)) + '\n')
    previous_mode = parent.stat().st_mode
    if os.name == 'nt':
        subprocess.run(['icacls', str(parent), '/deny', '*S-1-1-0:(AD)'], check=True, capture_output=True, text=True)
    else:
        parent.chmod(0o500)
    try:
        try:
            (parent / 'permission-probe').mkdir()
        except PermissionError:
            pass
        else:
            raise AssertionError('Test permissions did not prevent directory creation')
        failed = commands.run('move', '--overwrite', '--force', str(source), str(destination), 'Relocate document', exit_code=1)
        moved, deleted = failed['actions']
        assert moved['status'] == 3 and moved['decision'] == 'deny' and moved['resolvedAt']
        assert deleted['status'] == 6 and deleted['failure'] and deleted['resolvedAt']
        assert moved['blockerID'] == deleted['id'] and str(deleted['id']) in moved['failure']
        assert source.read_bytes() == destination.read_bytes() == commands.document.read_bytes()
        assert not trash.exists()
        assert commands.run('view', str(moved['id'])) == moved and commands.run('view', str(deleted['id'])) == deleted
        assert_notifications(listener, [])
    finally:
        if os.name == 'nt':
            subprocess.run(['icacls', str(parent), '/remove:d', '*S-1-1-0'], check=True, capture_output=True, text=True)
        else:
            parent.chmod(previous_mode)
        commands.configure([])
    print('PASS: failed trash creation preserves both items and records failed delete plus denied move')


def move_repeated_destination(commands, listener):
    # Kind: permanent. Independent moves use separate deletes in either approval order.
    for reverse in (False, True):
        sources = [commands.source(f'repeated-{reverse}-{index}') for index in range(2)]
        sources[1].write_bytes((Path(__file__).resolve().parents[1] / 'LICENSE').read_bytes())
        source_bytes = [source.read_bytes() for source in sources]
        destination = commands.directory / f'repeated-{reverse}-destination'
        occupant_bytes = (Path(__file__).resolve().parents[1] / 'go.mod').read_bytes()
        destination.write_bytes(occupant_bytes)
        commands.configure([str(destination)])
        pairs = [commands.run('move', '--overwrite', '--force', str(source), str(destination), 'Relocate document')['actions'] for source in sources]
        assert len({action['id'] for pair in pairs for action in pair}) == 4
        assert all(action['status'] == 2 for pair in pairs for action in pair)
        assert_notifications(listener, [action['id'] for pair in pairs for action in (pair[1], pair[0])])
        assert destination.read_bytes() == occupant_bytes
        order = [1, 0] if reverse else [0, 1]
        retained_bytes = occupant_bytes
        for index in order:
            moved, deleted = pairs[index]
            commands.run('dispose', str(deleted['id']), 'approve')
            assert commands.run('view', str(moved['id']))['status'] == 1
            assert destination.read_bytes() == source_bytes[index]
            assert (commands.directory / 'trash' / deleted['trashID']).read_bytes() == retained_bytes
            retained_bytes = source_bytes[index]
        assert all(not source.exists() for source in sources)
        assert_notifications(listener, [])
    print('PASS: separate occupying deletes preserve every displaced item in either approval order')


def move_recording_failure(commands, listener):
    # Kind: permanent. Failed pair saving retains the delete's recovery location and leaves the source untouched.
    commands.configure([])
    source = commands.source('failed-pair-source')
    destination = commands.directory / 'failed-pair-destination'
    occupant_bytes = (Path(__file__).resolve().parents[1] / 'LICENSE').read_bytes()
    destination.write_bytes(occupant_bytes)
    previous_history = commands.run('list')
    database_path = commands.directory / '.saferm' / 'saferm.sqlite'
    with sqlite3.connect(database_path) as database:
        database.execute("CREATE TRIGGER reject_pair BEFORE INSERT ON actions BEGIN SELECT RAISE(FAIL,'pair recording rejected'); END")
    try:
        failed = commands.invoke('move', '--overwrite', '--force', str(source), str(destination), 'Relocate document', exit_code=1)
        deleted = commands.printed_json(failed)
        assert deleted['id'] == 0 and not deleted.get('blockerID') and 'actions' not in deleted
        assert deleted['type'] == 'delete' and deleted['status'] == 1
        assert commands.catalog['error']['databaseResult'] in failed.stderr
        assert commands.run('list') == previous_history
        assert source.read_bytes() == commands.document.read_bytes() and not destination.exists()
        assert (commands.directory / 'trash' / deleted['trashID']).read_bytes() == occupant_bytes
        assert_notifications(listener, [])
    finally:
        with sqlite3.connect(database_path) as database:
            database.execute('DROP TRIGGER reject_pair')
    print('PASS: failed pair persistence exposes recoverable contents without saving either record')


def move_preparation(commands, listener):
    # Kind: permanent. Empty moves need no trash; invalid replacement and missing approval sources remain recoverable.
    source = commands.source('empty-source')
    destination = commands.directory / 'empty-destination'
    settings = commands.directory / '.saferm' / 'config.toml'
    settings.write_text('require-review = []\n')
    moved = commands.run('move', '--overwrite', str(source), str(destination), 'Relocate document')
    assert moved['status'] == 1 and not moved.get('blockerID')
    assert destination.read_bytes() == commands.document.read_bytes() and not source.exists()
    assert_notifications(listener, [])

    commands.configure([])
    directory = commands.directory / 'overlap-directory'
    directory.mkdir()
    source = commands.source('overlap-directory/source')
    previous_history = commands.run('list')
    refused = commands.invoke('move', '--overwrite', '--force', str(source), str(directory), 'Relocate document', exit_code=1)
    assert commands.catalog['error']['fileOverlap'] in refused.stderr
    assert source.read_bytes() == commands.document.read_bytes() and commands.run('list') == previous_history
    assert_notifications(listener, [])

    source = commands.source('missing-move-source')
    destination = commands.source('missing-move-destination')
    commands.configure([str(source)])
    pair = commands.run('move', '--overwrite', '--force', str(source), str(destination), 'Relocate document')
    moved, deleted = pair['actions']
    assert moved['status'] == 2 and deleted['status'] == 1
    assert_notifications(listener, [moved['id']])
    retained_source = source.with_suffix('.retained')
    source.rename(retained_source)
    previous_history = commands.run('list')
    refused = commands.invoke('dispose', str(moved['id']), 'approve', exit_code=1)
    assert commands.catalog['error']['fileInspect'] in refused.stderr
    assert commands.run('list') == previous_history
    denied = commands.run('dispose', str(moved['id']), 'deny')
    assert denied['id'] == moved['id'] and denied['status'] == 3 and denied['decision'] == 'deny'
    assert retained_source.read_bytes() == (commands.directory / 'trash' / deleted['trashID']).read_bytes() == commands.document.read_bytes()
    assert_notifications(listener, [])
    print('PASS: empty moves, refused preparation, and missing approval sources retain their required outcomes')


def replacement_trash_location(commands, listener):
    # Kind: permanent. Replacement refuses to relocate the trash that would retain its occupant.
    for operation in ('move', 'restore'):
        for alias_layout in ('direct', 'through source', 'outside source'):
            for trash_exists in (False, True):
                commands.configure([])
                case_name = f'{operation}-retained-trash-{trash_exists}-{alias_layout}'
                moving_directory = commands.directory / (case_name + '-source')
                moving_directory.mkdir()
                commands.source(moving_directory.name + '/document')
                occupant_path = commands.directory / (case_name + '-destination')
                if operation == 'restore':
                    original = commands.run('move', str(moving_directory), str(occupant_path), 'Relocate document directory')
                    moving_directory, occupant_path = occupant_path, moving_directory
                    arguments = ['restore', '--overwrite', '--force', str(original['id'])]
                else:
                    arguments = ['move', '--overwrite', '--force', str(moving_directory), str(occupant_path), 'Replace document directory']
                occupant_bytes = (Path(__file__).resolve().parents[1] / 'LICENSE').read_bytes()
                occupant_path.write_bytes(occupant_bytes)
                trash_directory = moving_directory / 'trash'
                if alias_layout == 'direct':
                    if trash_exists:
                        trash_directory.mkdir()
                else:
                    physical_trash = commands.directory / (case_name + '-trash')
                    physical_trash.mkdir()
                    intermediate_link = moving_directory / 'alias' if alias_layout == 'through source' else commands.directory / (case_name + '-alias')
                    intermediate_link.symlink_to(physical_trash, target_is_directory=True)
                    configured_link = commands.directory / (case_name + '-configured')
                    configured_link.symlink_to(intermediate_link, target_is_directory=True)
                    trash_directory = configured_link if trash_exists else configured_link / 'nested'
                settings = commands.directory / '.saferm' / 'config.toml'
                settings.write_text('trash-path = ' + json.dumps(str(trash_directory)) + '\nrequire-review = []\n')
                previous_history = commands.run('list')
                if alias_layout == 'outside source':
                    replaced = commands.run(*arguments)
                    deleted = next(action for action in replaced['actions'] if action['type'] == 'delete')
                    assert not moving_directory.exists()
                    assert (occupant_path / 'document').read_bytes() == commands.document.read_bytes()
                    assert (trash_directory / deleted['trashID']).read_bytes() == occupant_bytes
                    assert commands.run('view', str(deleted['id'])) == deleted
                    assert_notifications(listener, [])
                    continue
                refused = commands.invoke(*arguments, exit_code=1)
                assert not refused.stdout and commands.catalog['error']['fileOverlap'] in refused.stderr
                assert commands.run('list') == previous_history
                assert (moving_directory / 'document').read_bytes() == commands.document.read_bytes()
                assert occupant_path.read_bytes() == occupant_bytes
                if trash_exists:
                    assert list(trash_directory.iterdir()) == []
                else:
                    assert not trash_directory.exists()
                assert_notifications(listener, [])
    commands.configure([])
    print('PASS: move and restore preserve trash access through direct paths and symbolic-link chains')


def plain_output(commands, *arguments):
    """Run the binary without --json and return its completed process."""
    return subprocess.run([commands.binary, *arguments], env=commands.environment, capture_output=True, text=True, timeout=10)


def empty_builtin_list(commands):
    # Kind: permanent. With no configuration file, no pattern makes a delete wait for approval.
    assert not (commands.directory / '.saferm' / 'config.toml').exists()
    source = commands.source('.sample')
    delete_action = commands.run('delete', str(source), 'Remove sample document')
    assert delete_action['status'] == 1 and 'reviewRule' not in delete_action, delete_action
    assert not source.exists()
    assert (commands.directory / '.saferm' / 'trash' / delete_action['trashID']).read_bytes() == commands.document.read_bytes()
    default_document = plain_output(commands, 'config', '--default')
    assert default_document.returncode == 0 and tomllib.loads(default_document.stdout)['require-review'] == [], default_document
    config_display = plain_output(commands, 'config')
    review_line = commands.catalog['config']['review'].split() + [commands.catalog['config']['empty']]
    assert config_display.returncode == 0 and review_line in [line.split() for line in config_display.stdout.splitlines()], config_display
    print('PASS: the built-in configuration lists no review patterns')


def omitted_or_blank_trash_path(commands):
    # Kind: permanent. An omitted or blank trash-path uses the built-in trash directory for config,
    # delete, restore, and move replacement.
    built_in = commands.run('config', '--default')['trashPath']
    built_in_directory = Path(str(commands.directory) + built_in[1:]) if built_in.startswith('~') else Path(built_in)
    occupant_bytes = (Path(__file__).resolve().parents[1] / 'LICENSE').read_bytes()
    settings = commands.directory / '.saferm' / 'config.toml'
    settings.parent.mkdir(exist_ok=True)
    for label, document in (('omitted', 'require-review = []\n'),
                            ('empty', 'trash-path = ""\nrequire-review = []\n'),
                            ('blank', 'trash-path = "   "\nrequire-review = []\n')):
        settings.write_text(document)
        assert commands.run('config')['trashPath'] == built_in, label

        source = commands.source(label + '-trash-source')
        deleted = commands.run('delete', str(source), 'Remove document')
        assert deleted['status'] == 1 and Path(deleted['trashPath']) == built_in_directory, (label, deleted)
        assert (built_in_directory / deleted['trashID']).read_bytes() == commands.document.read_bytes() and not source.exists()
        restored = commands.run('restore', str(deleted['id']))
        assert restored['status'] == 4 and source.read_bytes() == commands.document.read_bytes(), (label, restored)

        destination = commands.directory / (label + '-trash-destination')
        destination.write_bytes(occupant_bytes)
        pair = commands.run('move', '--overwrite', '--force', str(source), str(destination), 'Relocate document')
        moved, occupying = pair['actions']
        assert moved['status'] == 1 and occupying['status'] == 1, (label, pair)
        assert (built_in_directory / occupying['trashID']).read_bytes() == occupant_bytes, label
        assert destination.read_bytes() == commands.document.read_bytes() and not source.exists(), label
    settings.unlink()
    print('PASS: an omitted or blank trash-path uses the built-in trash directory')


def omitted_retain_period(shared_commands):
    # Kind: permanent. An omitted retain-period uses the built-in retention period for config and cleanup;
    # an omitted require-review stays unset. A separate home keeps earlier interrupted records out of cleanup.
    commands = Commands(shared_commands.binary, shared_commands.directory / 'retention-home', shared_commands.document)
    built_in_days = commands.run('config', '--default')['retainPeriod']
    settings = commands.directory / '.saferm' / 'config.toml'
    settings.parent.mkdir(exist_ok=True)
    settings.write_text('trash-path = "~/.saferm/trash"\n')
    configuration = commands.run('config')
    assert configuration['retainPeriod'] == built_in_days and configuration['requireReview'] is None, configuration

    restored_ids = []
    for name in ('expired-retention-document', 'retained-retention-document'):
        deleted = commands.run('delete', str(commands.source(name)), 'Remove document')
        assert commands.run('restore', str(deleted['id']))['status'] == 4
        restored_ids.append(deleted['id'])
    expired_id, retained_id = restored_ids
    now = datetime.now(timezone.utc)
    with sqlite3.connect(commands.directory / '.saferm' / 'saferm.sqlite') as database:
        for action_id, age in ((expired_id, timedelta(days=built_in_days, hours=1)),
                               (retained_id, timedelta(days=built_in_days, hours=-1))):
            database.execute('UPDATE actions SET resolved_at=? WHERE action_id=?',
                             ((now - age).strftime('%Y-%m-%dT%H:%M:%S.%fZ'), action_id))

    cleaned = commands.run('cleanup')
    assert expired_id in cleaned.get('removed', []) and retained_id not in cleaned.get('removed', []), cleaned
    assert commands.run('view', str(retained_id))['id'] == retained_id
    commands.invoke('view', str(expired_id), exit_code=1)
    settings.unlink()
    print('PASS: an omitted retain-period uses the built-in retention period; an omitted require-review stays unset')


TERMINAL_RED = '\x1b[31m'


def invalid_value_text(commands, value):
    """Return the catalog error for a replaced value with all whitespace removed, since the display wraps it."""
    return ''.join(commands.catalog['config']['invalidValue'].replace('{{.}}', value).split())


def assert_invalid_value_shown(commands, value):
    """Check that the piped text display of config shows the error for the value, without color codes."""
    display = plain_output(commands, 'config')
    assert display.returncode == 0, display
    assert invalid_value_text(commands, value) in ''.join(display.stdout.split()), display.stdout
    assert '\x1b[' not in display.stdout, display.stdout


def terminal_output(commands, *arguments):
    """Run the binary with standard output on a pseudo-terminal and return what it wrote there."""
    leader, follower = pty.openpty()
    process = subprocess.Popen([commands.binary, *arguments], env=commands.environment, stdout=follower,
                               stderr=subprocess.DEVNULL)
    os.close(follower)
    written = b''
    while True:
        try:
            chunk = os.read(leader, 65536)
        except OSError:
            break
        if not chunk:
            break
        written += chunk
    os.close(leader)
    assert process.wait(timeout=10) == 0, written
    return written.decode()


def invalid_retain_period(shared_commands):
    # Kind: permanent. A retain-period that can't be applied is replaced by the built-in one,
    # config reports both values, and cleanup succeeds. A separate home keeps earlier interrupted records out of cleanup.
    commands = Commands(shared_commands.binary, shared_commands.directory / 'invalid-retention-home', shared_commands.document)
    built_in_days = commands.run('config', '--default')['retainPeriod']
    settings = commands.directory / '.saferm' / 'config.toml'
    settings.parent.mkdir(exist_ok=True)
    for written, shown in (('-5', '-5'), ('"thirty"', 'thirty'), ('1.5', '1.5')):
        settings.write_text('retain-period = ' + written + '\nrequire-review = []\n')
        configuration = commands.run('config')
        assert configuration['retainPeriod'] == built_in_days and configuration['invalidRetainPeriod'] == shown, configuration
        assert 'invalidTrashPath' not in configuration, configuration
        assert_invalid_value_shown(commands, shown)
        commands.run('cleanup')
    settings.unlink()
    print('PASS: a retain-period that cannot be applied is replaced by the built-in one and reported by config')


def invalid_trash_path(commands):
    # Kind: permanent. A trash-path that can't be applied is replaced by the built-in trash directory
    # for config, delete, move replacement, and restore replacement; the file in the way is untouched.
    built_in = commands.run('config', '--default')['trashPath']
    built_in_directory = Path(str(commands.directory) + built_in[1:]) if built_in.startswith('~') else Path(built_in)
    occupant_bytes = (Path(__file__).resolve().parents[1] / 'LICENSE').read_bytes()
    settings = commands.directory / '.saferm' / 'config.toml'
    settings.parent.mkdir(exist_ok=True)

    settings.write_text('trash-path = 5\nrequire-review = []\n')
    configuration = commands.run('config')
    assert configuration['trashPath'] == built_in and configuration['invalidTrashPath'] == '5', configuration
    assert_invalid_value_shown(commands, '5')

    for nested in (False, True):
        label = 'nested-file-trash' if nested else 'file-trash'
        blocking_file = commands.source(label + '-blocker')
        configured = str(blocking_file / 'missing' / 'nested' if nested else blocking_file)
        settings.write_text('trash-path = ' + json.dumps(configured) + '\nrequire-review = []\n')
        configuration = commands.run('config')
        assert configuration['trashPath'] == built_in and configuration['invalidTrashPath'] == configured, configuration
        assert 'invalidRetainPeriod' not in configuration, configuration
        assert_invalid_value_shown(commands, configured)

        source = commands.source(label + '-source')
        deleted = commands.run('delete', str(source), 'Remove document')
        assert deleted['status'] == 1 and Path(deleted['trashPath']) == built_in_directory, (label, deleted)
        assert (built_in_directory / deleted['trashID']).read_bytes() == commands.document.read_bytes()

        source.write_bytes(occupant_bytes)
        restored, occupying = commands.run('restore', '--overwrite', '--force', str(deleted['id']))['actions'][::-1]
        assert restored['status'] == 4 and occupying['status'] == 1, (label, restored, occupying)
        assert source.read_bytes() == commands.document.read_bytes()
        assert (built_in_directory / occupying['trashID']).read_bytes() == occupant_bytes

        destination = commands.directory / (label + '-destination')
        destination.write_bytes(occupant_bytes)
        moved, replaced = commands.run('move', '--overwrite', '--force', str(source), str(destination), 'Replace document')['actions']
        assert moved['status'] == 1 and replaced['status'] == 1, (label, moved, replaced)
        assert (built_in_directory / replaced['trashID']).read_bytes() == occupant_bytes
        assert destination.read_bytes() == commands.document.read_bytes() and not source.exists()
        assert blocking_file.read_bytes() == commands.document.read_bytes(), label
    settings.unlink()
    print('PASS: a trash-path that cannot be applied is replaced by the built-in trash directory and reported by config')


def terminal_error_color(commands):
    # Kind: permanent. On a terminal, config shows a replaced value's error in red; valid settings show no red.
    settings = commands.directory / '.saferm' / 'config.toml'
    settings.parent.mkdir(exist_ok=True)
    settings.write_text('retain-period = -5\nrequire-review = []\n')
    displayed = terminal_output(commands, 'config')
    assert TERMINAL_RED in displayed and invalid_value_text(commands, '-5') in ''.join(displayed.replace(TERMINAL_RED, '').replace('\x1b[0m', '').split()), displayed
    settings.write_text('retain-period = 5\nrequire-review = []\n')
    assert TERMINAL_RED not in terminal_output(commands, 'config')
    settings.unlink()
    print('PASS: config shows a replaced value in red on a terminal and no red for valid settings')


def place(commands, relative_path):
    """Copy the supplied document to a path under the test home, creating its parents."""
    (commands.directory / relative_path).parent.mkdir(parents=True, exist_ok=True)
    return commands.source(relative_path)


def assert_pending(commands, pattern, *arguments):
    """Run a delete or move that must wait for approval and record the pattern."""
    source = Path(arguments[1])
    link_target = source.readlink() if source.is_symlink() else None
    source_bytes = None if link_target or source.is_dir() else source.read_bytes()
    action = commands.run(*arguments)
    assert action['status'] == 2 and action.get('reviewRule') == pattern, (pattern, action)
    assert source.is_symlink() == bool(link_target) and (source.is_symlink() or source.exists()), source
    if link_target:
        assert source.readlink() == link_target, source
    elif source_bytes is not None:
        assert source.read_bytes() == source_bytes, source
    if arguments[0] == 'move':
        assert not Path(arguments[2]).exists(), arguments[2]
    return action


def assert_completed(commands, *arguments):
    """Run a delete or move that no pattern protects; its source must be gone."""
    source = Path(arguments[1])
    action = commands.run(*arguments)
    assert action['status'] == 1 and 'reviewRule' not in action, action
    assert not source.exists() and not source.is_symlink(), source
    return action


def name_and_path_patterns(commands):
    # Kind: permanent. A pattern with no slash before its last character matches a name in any
    # folder; one with a slash applies from the filesystem root; one that starts with ~ from home.
    commands.configure(['*.pem'])
    assert_pending(commands, '*.pem', 'delete', str(place(commands, 'kinds/keys/a.pem')), 'Remove key')
    assert_completed(commands, 'delete', str(place(commands, 'kinds/keys/a.pem.bak')), 'Remove backup')
    rooted_pattern = str(commands.directory / 'kinds' / 'build' / 'output').lstrip('/') + '/'
    commands.configure([rooted_pattern])
    destination = commands.directory / 'kinds' / 'moved-output-file'
    assert_pending(commands, rooted_pattern, 'move', str(place(commands, 'kinds/build/output/f')), str(destination), 'Relocate output')
    assert_completed(commands, 'delete', str(place(commands, 'kinds/elsewhere/build/output/f')), 'Remove output')
    commands.configure(['~/kinds/*.txt'])
    assert_pending(commands, '~/kinds/*.txt', 'delete', str(place(commands, 'kinds/a.txt')), 'Remove text')
    assert_completed(commands, 'delete', str(place(commands, 'kinds/notes/a.txt')), 'Remove nested text')
    print('PASS: name patterns, root patterns, and home patterns apply where their kind says')


def folder_contents_protected(commands):
    # Kind: permanent. A pattern that matches a folder protects everything inside it.
    commands.configure(['~/vault/'])
    assert_pending(commands, '~/vault/', 'delete', str(place(commands, 'vault/deep/file')), 'Remove vault file')
    assert_completed(commands, 'delete', str(place(commands, 'vault-other/deep/file')), 'Remove other file')
    print('PASS: a matched folder protects the items inside it at any depth')


def bracket_folder_name(commands):
    # Kind: permanent. A folder whose real name holds [ is protected under exactly that name, by a
    # pattern that escapes the bracket and by a pattern written through a link to the folder.
    album_link = commands.directory / 'photos' / 'alias'
    album_link.parent.mkdir()
    album_link.symlink_to(commands.directory / 'photos' / 'Album[1]', target_is_directory=True)
    for pattern in ('~/photos/Album\\[1\\]/private/**', '~/photos/alias/private/**'):
        commands.configure([pattern])
        assert_pending(commands, pattern, 'delete', str(place(commands, 'photos/Album[1]/private/f')), 'Remove photo')
        assert_completed(commands, 'delete', str(place(commands, 'photos/Album1/private/f')), 'Remove other photo')
    print('PASS: a folder with a bracket in its name is protected under exactly that name')


def linked_patterns(commands):
    # Kind: permanent. A pattern that spells out a link's path protects the link and its target; a
    # pattern that matches the link through a wildcard protects the link alone; a pattern that ends
    # in a slash matches a link that points to a folder.
    documents_folder = commands.directory / 'My Documents'
    linked_document = place(commands, 'My Documents/reports/x')
    documents_link = commands.directory / 'MyDocuments'
    documents_link.symlink_to(documents_folder, target_is_directory=True)
    commands.configure(['~/MyDocuments/'])
    assert_pending(commands, '~/MyDocuments/', 'delete', str(linked_document), 'Remove linked document')
    assert_pending(commands, '~/MyDocuments/', 'delete', str(documents_link), 'Remove the link')
    assert_completed(commands, 'delete', str(place(commands, 'Other Documents/reports/x')), 'Remove other document')
    commands.configure(['~/MyDocument?/'])
    assert_pending(commands, '~/MyDocument?/', 'delete', str(documents_link), 'Remove the link')
    assert_completed(commands, 'delete', str(linked_document), 'Remove linked document')
    commands.configure(['.git/'])
    place(commands, 'link-targets/repository/config')
    folder_link = commands.directory / 'work' / '.git'
    folder_link.parent.mkdir()
    folder_link.symlink_to(commands.directory / 'link-targets' / 'repository', target_is_directory=True)
    assert_pending(commands, '.git/', 'delete', str(folder_link), 'Remove repository link')
    file_link = commands.directory / 'plain' / '.git'
    file_link.parent.mkdir()
    file_link.symlink_to(place(commands, 'link-targets/file'))
    assert_completed(commands, 'delete', str(file_link), 'Remove file link')
    assert (commands.directory / 'link-targets' / 'file').read_bytes() == commands.document.read_bytes()
    print('PASS: a pattern through a link protects the link, its target, and links to folders')


def letter_case(commands):
    # Kind: permanent. Letter case matters for a name exactly when its folder tells two spellings apart.
    probe_folder = commands.directory / 'spellings'
    probe_folder.mkdir()
    for probe_name in ('probe', 'PROBE'):
        (probe_folder / probe_name).write_text(probe_name)
    ignores_case = len(list(probe_folder.iterdir())) == 1
    commands.configure(['~/Notes/'])
    exact_note = place(commands, 'Notes/exact.txt')
    assert_pending(commands, '~/Notes/', 'delete', str(exact_note), 'Remove note')
    respelled_note = commands.directory / 'notes' / 'respelled.txt'
    if ignores_case:
        place(commands, 'Notes/respelled.txt')
        assert_pending(commands, '~/Notes/', 'delete', str(respelled_note), 'Remove respelled note')
    else:
        assert_completed(commands, 'delete', str(place(commands, 'notes/respelled.txt')), 'Remove respelled note')
    assert_completed(commands, 'delete', str(place(commands, 'Nodes/other.txt')), 'Remove other note')
    print('PASS: letter case follows the folder that holds the name (ignores case: ' + str(ignores_case) + ')')


def recorded_pattern(commands):
    # Kind: permanent. A pending action records the first listed pattern that protects its source.
    for patterns, first_protecting_pattern in ((['*.pem', '~/order/vault/'], '*.pem'), (['~/order/vault/', '*.pem'], '~/order/vault/')):
        commands.configure(patterns)
        assert_pending(commands, first_protecting_pattern, 'delete', str(place(commands, 'order/vault/key.pem')), 'Remove key')
    commands.configure(['*.txt', 'notes*'])
    assert_pending(commands, '*.txt', 'delete', str(place(commands, 'order/notes.txt')), 'Remove notes')
    assert_completed(commands, 'delete', str(place(commands, 'order/draft.md')), 'Remove draft')
    print('PASS: list order decides which protecting pattern is recorded')


def folder_requests(commands):
    # Kind: permanent. A delete or move of a folder that contains a protected item needs approval,
    # and records the first listed pattern that protects an item inside the folder.
    commands.configure(['.git/'])
    project_folder = commands.directory / 'contents' / 'proj'
    repository_file = place(commands, 'contents/proj/.git/config')
    assert_pending(commands, '.git/', 'delete', str(project_folder), 'Remove project')
    archive_folder = commands.directory / 'contents' / 'archive'
    archive_folder.mkdir()
    assert_pending(commands, '.git/', 'move', str(project_folder), str(archive_folder / 'proj'), 'Archive project')
    commands.configure(['*.pem', '.git/'])
    key_file = place(commands, 'contents/proj/deep/key.pem')
    assert_pending(commands, '*.pem', 'delete', str(project_folder), 'Remove project')
    for kept_file in (repository_file, key_file):
        assert kept_file.read_bytes() == commands.document.read_bytes(), kept_file
    place(commands, 'contents/plain/readme')
    assert_completed(commands, 'delete', str(commands.directory / 'contents' / 'plain'), 'Remove plain folder')
    print('PASS: a folder request waits when the folder contains a protected item')


def unfollowed_link(commands):
    # Kind: permanent. A link inside a deleted folder is judged as a link and is not followed.
    commands.configure(['.git/'])
    repository_file = place(commands, 'links/proj/.git/config')
    linker_folder = commands.directory / 'links' / 'linker'
    linker_folder.mkdir()
    (linker_folder / 'ref').symlink_to(commands.directory / 'links' / 'proj', target_is_directory=True)
    delete_action = assert_completed(commands, 'delete', str(linker_folder), 'Remove linker')
    assert (commands.directory / 'trash' / delete_action['trashID'] / 'ref').is_symlink()
    assert repository_file.read_bytes() == commands.document.read_bytes()
    print('PASS: a link inside a deleted folder is not followed')


def unlock_folders(directory):
    """Give back the owner permissions of every folder under the directory that has none."""
    for parent, folder_names, _ in os.walk(directory):
        for folder_name in folder_names:
            folder = Path(parent) / folder_name
            if not folder.is_symlink() and folder.stat().st_mode & 0o700 == 0:
                folder.chmod(0o700)


def unreadable_folders(commands):
    # Kind: permanent. An unreadable folder holds a request only when some pattern could apply
    # inside it and no visible item is protected; SafeRM then warns about the folder, in the
    # text after the action and in the problems list of the JSON result.
    try:
        for patterns, case_name in (([], 'none'), ([''], 'empty'), (['~/Contracts/'], 'elsewhere')):
            commands.configure(patterns)
            source = commands.directory / 'unreadable' / case_name / 'old'
            place(commands, f'unreadable/{case_name}/old/a.txt')
            (source / 'locked').mkdir()
            (source / 'locked').chmod(0)
            delete_action = assert_completed(commands, 'delete', str(source), 'Remove old downloads')
            assert (commands.directory / 'trash' / delete_action['trashID'] / 'a.txt').read_bytes() == commands.document.read_bytes()
        commands.configure(['*.pem'])
        for case_name, key_is_visible, json_requested in (('visible', True, False), ('hidden', False, False),
                                                      ('visible-json', True, True), ('hidden-json', False, True)):
            source = commands.directory / 'unreadable' / case_name / 'old'
            place(commands, f'unreadable/{case_name}/old/a.txt')
            if key_is_visible:
                place(commands, f'unreadable/{case_name}/old/a.pem')
            locked_folder = source / 'locked'
            locked_folder.mkdir()
            locked_folder.chmod(0)
            warning = commands.catalog['warning']['unreadable'].replace('%q', json.dumps(str(locked_folder)))
            if json_requested:
                delete_output = json.loads(commands.invoke('delete', str(source), 'Remove old downloads').stdout)
                assert [action['source'] for action in delete_output['actions']] == [str(source)], delete_output
                assert delete_output.get('problems') == (None if key_is_visible else [warning]), delete_output
            else:
                command_process = plain_output(commands, 'delete', str(source), 'Remove old downloads')
                assert command_process.returncode == 0 and (warning in command_process.stdout.splitlines()) != key_is_visible, command_process
            pending_actions = commands.run('list', '--status', 'pending')
            assert pending_actions and pending_actions[-1]['source'] == str(source), pending_actions
            pending_action = pending_actions[-1]
            assert pending_action['reviewRule'] == '*.pem', pending_action
            assert (source / 'a.txt').read_bytes() == commands.document.read_bytes()
            assert locked_folder.is_dir() and locked_folder.stat().st_mode & 0o777 == 0
            approved_action = commands.run('dispose', str(pending_action['id']), 'approve')
            assert approved_action['status'] == 1 and not source.exists(), approved_action
            assert (commands.directory / 'trash' / approved_action['trashID'] / 'a.txt').read_bytes() == commands.document.read_bytes()
    finally:
        unlock_folders(commands.directory)
    print('PASS: an unreadable folder holds a request only where a pattern could apply inside it')


def occupant_contents(commands):
    # Kind: permanent. The occupying delete of a move or restore replacement checks what the occupant contains.
    commands.configure(['*.pem'])
    unprotected_moving_folder = place(commands, 'occupied/plain-moving/document').parent
    unprotected_occupant_folder = place(commands, 'occupied/plain-occupant/notes/a.txt').parents[1]
    replacement_result = commands.run('move', '--overwrite', '--force', str(unprotected_moving_folder),
                                      str(unprotected_occupant_folder), 'Replace folder')
    move_action, delete_action = replacement_result['actions']
    assert move_action['status'] == delete_action['status'] == 1 and 'reviewRule' not in delete_action, replacement_result
    assert not unprotected_moving_folder.exists()
    assert (unprotected_occupant_folder / 'document').read_bytes() == commands.document.read_bytes()
    assert (commands.directory / 'trash' / delete_action['trashID'] / 'notes' / 'a.txt').read_bytes() == commands.document.read_bytes()
    moving_folder = place(commands, 'occupied/moving/document').parent
    occupant_folder = place(commands, 'occupied/occupant/keys/a.pem').parents[1]
    replacement_result = commands.run('move', '--overwrite', '--force', str(moving_folder), str(occupant_folder), 'Replace folder')
    move_action, delete_action = replacement_result['actions']
    assert move_action['status'] == 2 and 'reviewRule' not in move_action and move_action['blockerID'] == delete_action['id'], replacement_result
    assert delete_action['status'] == 2 and delete_action['reviewRule'] == '*.pem', replacement_result
    assert (moving_folder / 'document').read_bytes() == (occupant_folder / 'keys' / 'a.pem').read_bytes() == commands.document.read_bytes()
    commands.configure([])
    unprotected_original_folder = place(commands, 'occupied/plain-original/document').parent
    unprotected_original_action = commands.run('delete', str(unprotected_original_folder), 'Retain original folder')
    place(commands, 'occupied/plain-original/notes/a.txt')
    original_path = place(commands, 'occupied/original/document').parent
    original_action = commands.run('delete', str(original_path), 'Retain original folder')
    place(commands, 'occupied/original/keys/a.pem')
    commands.configure(['*.pem'])
    restoration_result = commands.run('restore', '--overwrite', '--force', str(unprotected_original_action['id']))
    occupant_delete, restored_action = restoration_result['actions']
    assert occupant_delete['type'] == 'delete' and occupant_delete['status'] == 1 and 'reviewRule' not in occupant_delete, restoration_result
    assert restored_action['id'] == unprotected_original_action['id'] and restored_action['status'] == 4, restoration_result
    assert (unprotected_original_folder / 'document').read_bytes() == commands.document.read_bytes()
    assert not (unprotected_original_folder / 'notes').exists()
    assert (commands.directory / 'trash' / occupant_delete['trashID'] / 'notes' / 'a.txt').read_bytes() == commands.document.read_bytes()
    command_process = commands.invoke('restore', '--overwrite', '--force', str(original_action['id']), exit_code=1)
    pending_delete = commands.printed_json(command_process)
    assert pending_delete['type'] == 'delete' and pending_delete['status'] == 2 and pending_delete['reviewRule'] == '*.pem'
    error_text = commands.catalog['error']['restorePendingDetail'].replace('%w', commands.catalog['error']['restorePending'])
    assert json.loads(command_process.stderr)['error'] == error_text % pending_delete['id'], command_process.stderr
    assert (original_path / 'keys' / 'a.pem').read_bytes() == commands.document.read_bytes()
    assert commands.run('view', str(original_action['id'])) == original_action
    print('PASS: replacement checks the contents of the occupant it would delete')


def malformed_pattern(commands):
    # Kind: permanent. While the list holds a malformed pattern, every delete is saved pending and
    # reported with an error that names the pattern; config reports the same error.
    error_text = (commands.catalog['error']['ruleMalformedDetail']
                  .replace('%q', json.dumps('[abc')).replace('%w', commands.catalog['error']['ruleMalformed']))
    commands.configure(['[abc'])
    notes_file = place(commands, 'malformed/notes.txt')
    command_process = commands.invoke('delete', str(notes_file), 'Remove notes', exit_code=1)
    assert command_process.stdout and json.loads(command_process.stderr) == {'error': error_text}, command_process
    pending_action = commands.printed_json(command_process)
    assert pending_action['status'] == 2 and pending_action['reviewRule'] == '[abc', pending_action
    assert commands.run('view', str(pending_action['id'])) == pending_action
    text_delete = plain_output(commands, 'delete', str(notes_file), 'Remove notes')
    assert text_delete.returncode == 1 and text_delete.stderr.splitlines() == [error_text], text_delete
    assert len([action for action in commands.run('list', '--status', 'pending') if action['source'] == str(notes_file)]) == 2
    assert notes_file.read_bytes() == commands.document.read_bytes()
    commands.configure(['*.pem', '[abc'])
    command_process = commands.invoke('delete', str(place(commands, 'malformed/a.pem')), 'Remove key', exit_code=1)
    assert commands.printed_json(command_process)['reviewRule'] == '*.pem' and json.loads(command_process.stderr) == {'error': error_text}, command_process
    config_display = plain_output(commands, 'config')
    review_line = commands.catalog['config']['review'].split() + ['*.pem,', '[abc']
    assert config_display.returncode == 1 and config_display.stderr.splitlines() == [error_text], config_display
    assert review_line in [line.split() for line in config_display.stdout.splitlines()], config_display.stdout
    command_process = commands.invoke('config', exit_code=1)
    assert json.loads(command_process.stdout)['requireReview'] == ['*.pem', '[abc'], command_process
    assert json.loads(command_process.stderr) == {'error': error_text}, command_process
    commands.configure(['*.pem'])
    config_display = plain_output(commands, 'config')
    assert config_display.returncode == 0 and not config_display.stderr and config_display.stdout, config_display
    assert_completed(commands, 'delete', str(notes_file), 'Remove notes')
    print('PASS: a malformed pattern holds every delete and is reported by requests and by config')


def action_lists(commands):
    # Kind: permanent. With --json, a delete, move, dispose, or restore prints a result whose
    # actions list holds every action, also when it holds one; view prints one action object and
    # list prints a list of actions.
    def listed_action(*arguments):
        """Run a command that must print a result holding only a list of one action; return the action."""
        printed_result = json.loads(commands.invoke(*arguments).stdout)
        assert list(printed_result) == ['actions'] and len(printed_result['actions']) == 1, (arguments, printed_result)
        return printed_result['actions'][0]

    commands.configure([])
    source = commands.directory / 'shapes' / 'document'
    destination = commands.directory / 'shapes' / 'moved-document'
    delete_action = listed_action('delete', str(place(commands, 'shapes/document')), 'Remove document')
    assert delete_action['type'] == 'delete' and delete_action['status'] == 1 and not source.exists(), delete_action
    restored_action = listed_action('restore', str(delete_action['id']))
    assert restored_action['id'] == delete_action['id'] and restored_action['status'] == 4 and source.exists(), restored_action
    move_action = listed_action('move', str(source), str(destination), 'Relocate document')
    assert move_action['type'] == 'move' and move_action['status'] == 1 and destination.exists(), move_action
    commands.configure([str(destination)])
    pending_action = listed_action('delete', str(destination), 'Remove relocated document')
    assert pending_action['status'] == 2 and destination.exists(), pending_action
    denied_action = listed_action('dispose', str(pending_action['id']), 'deny')
    assert denied_action['id'] == pending_action['id'] and denied_action['status'] == 3 and destination.exists(), denied_action
    assert json.loads(commands.invoke('view', str(denied_action['id'])).stdout) == denied_action
    denied_actions = json.loads(commands.invoke('list', '--status', 'denied').stdout)
    assert isinstance(denied_actions, list) and denied_action in denied_actions, denied_actions
    print('PASS: delete, move, dispose, and restore print their actions in a list, also for one action')


def assert_notifications(listener, action_ids):
    """Read saved request notifications in order and reject any extra notification."""
    for action_id in action_ids:
        listener.settimeout(1)
        connection, _ = listener.accept()
        with connection:
            message = json.load(connection.makefile())
        assert message == {'id': action_id, 'token': 'test-token'}, message
    listener.settimeout(0.05)
    try:
        connection, _ = listener.accept()
    except TimeoutError:
        return
    with connection:
        raise AssertionError('Unexpected pending notification: ' + str(json.load(connection.makefile())))


if __name__ == '__main__':
    commands = Commands(*sys.argv[1:])
    baseline(commands)
    occupied_move_refusal(commands)
    completed_record(commands)
    request_push(commands)
    unreused_ids(commands)
    restore_preservation(commands)
    restore_review(commands)
    restore_recording_failures(commands)
    commands.configure([])
    with socket.socket() as listener:
        listener.bind(('127.0.0.1', 0))
        listener.listen()
        address = '127.0.0.1:' + str(listener.getsockname()[1])
        (commands.directory / '.saferm' / 'notifications.json').write_text(json.dumps({'address': address, 'token': 'test-token'}))
        move_replacement(commands, listener)
        move_review(commands, listener)
        move_linked_decisions(commands, listener)
        move_delete_denial(commands, listener)
        move_transfer_failure(commands, listener)
        move_repeated_destination(commands, listener)
        move_recording_failure(commands, listener)
        move_preparation(commands, listener)
        replacement_trash_location(commands, listener)
    # The next case runs with no configuration file, and the cases after it send no notifications.
    (commands.directory / '.saferm' / 'config.toml').unlink()
    (commands.directory / '.saferm' / 'notifications.json').unlink()
    empty_builtin_list(commands)
    omitted_or_blank_trash_path(commands)
    omitted_retain_period(commands)
    invalid_retain_period(commands)
    invalid_trash_path(commands)
    terminal_error_color(commands)
    name_and_path_patterns(commands)
    folder_contents_protected(commands)
    bracket_folder_name(commands)
    linked_patterns(commands)
    letter_case(commands)
    recorded_pattern(commands)
    folder_requests(commands)
    unfollowed_link(commands)
    unreadable_folders(commands)
    occupant_contents(commands)
    malformed_pattern(commands)
    action_lists(commands)
    execution_before_recording(commands)
