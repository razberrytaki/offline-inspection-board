import { expect, test as base } from '@playwright/test';

export const test = base.extend<{ resetFaults: void }>({
  resetFaults: [async ({ request }, use) => {
    async function reset() {
      const response = await request.post('/api/dev/faults', { data: { offline: false, dropAfterCommit: 0 } });
      expect(response.status()).toBe(204);
    }
    await reset();
    try {
      await use();
    } finally {
      await reset();
    }
  }, { auto: true }]
});
