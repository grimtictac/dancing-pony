// Starts Scalar's API reference. A separate file, not an inline script, so
// the page's Content-Security-Policy can allow scripts from this origin only.
const config = {
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
  // After a successful login, use its token for every request, so trying
  // the API is "send Log in, then anything". The token stays in this tab.
  onResponseReceived: async ({ response, request }) => {
    if (!response.ok || !new URL(request.url).pathname.endsWith('/auth/login')) return;
    const { token } = await response.clone().json();
    if (!token) return;
    const current = reference.getConfiguration();
    reference.updateConfiguration({
      ...current,
      authentication: {
        ...current.authentication,
        preferredSecurityScheme: 'bearer',
        securitySchemes: { bearer: { token } },
      },
    });
  },
};
const reference = Scalar.createApiReference('#app', config);
