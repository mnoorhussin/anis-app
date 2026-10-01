import { readFileSync } from 'node:fs';
import { expect, it } from 'vitest';
import { REFUSAL_TEMPLATE } from '@anis/types';

it('ships the Arabic-first host refusal unchanged in the backend', () => {
  const go = readFileSync(
    new URL('../../../pocketbase/internal/prompt/grounding.go', import.meta.url),
    'utf8',
  );
  expect(go).toContain(`RefusalAR = "${REFUSAL_TEMPLATE.ar}"`);
  const english = go.match(/RefusalEN = "([^"]+)" \+\s*"([^"]+)"/);
  expect(english?.slice(1).join('')).toBe(REFUSAL_TEMPLATE.en);
  expect(REFUSAL_TEMPLATE.ar).toBe(
    'لم أجد هذه المعلومة في مصادر الشركة — هل أحوّلك إلى أحد الموظفين؟',
  );
});
