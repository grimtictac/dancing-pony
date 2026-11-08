// Starts Scalar's API reference. A separate file, not an inline script, so
// the page's Content-Security-Policy can allow scripts from this origin only.
Scalar.createApiReference('#app', {
  url: '/openapi.yaml',
  // Everything is served from here: no fonts from Scalar's CDN, no
  // telemetry. The CSP would block both anyway.
  withDefaultFonts: false,
  telemetry: false,
  hideDownloadButton: false,
  defaultOpenAllTags: false,
  persistAuth: false,
  // Scalar's toolbar ("Deploy API", "Generate SDK") and AI assistant are
  // for their hosted product; this page only documents the API.
  showDeveloperTools: 'never',
  agent: { disabled: true },
  authentication: { preferredSecurityScheme: 'bearer' },
});
