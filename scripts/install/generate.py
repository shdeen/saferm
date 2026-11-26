"""Generate standalone installers with their user-facing messages embedded."""
import argparse
import json
from pathlib import Path
import shlex


def generate(check_only):
    installer_directory = Path(__file__).parent
    message_catalog = json.loads((installer_directory / 'copy.json').read_text())
    for extension, language in [('sh', 'shell'), ('ps1', 'powershell')]:
        if language == 'shell':
            message_definitions = '\n'.join(f'msg_{message_name}={shlex.quote(message)}' for message_name, message in message_catalog[language].items())
        else:
            message_definitions = '$messages = @{\n' + '\n'.join(
                "    " + message_name + " = '" + message.replace("'", "''") + "'" for message_name, message in message_catalog[language].items()) + '\n}'
        installer_template = (installer_directory / f'install.{extension}.in').read_text()
        if installer_template.count('@@MESSAGES@@') != 1:
            raise ValueError(f'Expected one message insertion point in {extension} template')
        installer_text = installer_template.replace('@@MESSAGES@@', message_definitions)
        installer_path = installer_directory / f'install.{extension}'
        if check_only:
            if installer_path.read_text() != installer_text:
                raise ValueError(f'Regenerate {installer_path} with python3 scripts/install/generate.py')
        else:
            installer_path.write_text(installer_text, encoding='utf-8', newline='\n')


if __name__ == '__main__':
    argument_parser = argparse.ArgumentParser(description=__doc__)
    argument_parser.add_argument('--check', dest='check_only', action='store_true')
    generate(argument_parser.parse_args().check_only)
