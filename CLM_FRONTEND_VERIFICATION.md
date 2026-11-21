# CLM Frontend Verification

## Browser checks

- Admin portal loaded at `http://127.0.0.1:3000/`.
- Gateway portal loaded at `http://127.0.0.1:8080/`.
- Both initial page loads had no browser console errors and no failed requests.
- Admin demo credentials resulted in a populated dashboard.
- Admin browser emitted `Could not read role from token; treating as viewer`; source inspection identifies the demo server's fixed non-JWT mock token as the cause.
- Gateway portal rendered the login form and navigation; Playwright and browser click operations did not dispatch a stable click on Sign In within the timeout.

## Limitations

- A full menu/button sweep was not possible in the available browser session.
- Database-backed UI rendering was blocked by missing `.env.demo.local`.

