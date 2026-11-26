const port = Number(process.env.E2E_PORT ?? 18091);
export const demoURL = 'http://127.0.0.1:' + port;
export const normalURL = 'http://127.0.0.1:' + (port + 1);
