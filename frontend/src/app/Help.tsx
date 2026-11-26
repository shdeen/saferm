import { Stack, Text, Title } from '@mantine/core';
import { appstrings } from '../shared/text';

/** Shows the help sections from the product copy. */
export function Help() {
  return (
    <section className="help-panel">
      <Title order={1} mb="xl">
        {appstrings.helpTitle}
      </Title>
      <Stack gap="xl">
        {appstrings.helpSections.map(function helpSection(section) {
          return (
            <div key={section.title}>
              <Title order={3} size="h4" mb="xs">
                {section.title}
              </Title>
              <Text size="sm" c="dimmed" lh={1.7}>
                {section.text}
              </Text>
            </div>
          );
        })}
      </Stack>
    </section>
  );
}
