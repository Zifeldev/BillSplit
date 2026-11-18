/**
 * Dynamic layer over app.json. The universal-links / App Links domain defaults
 * to the one in app.json (billsplit.example.com) and can be overridden without
 * editing files:
 *
 *   APP_LINK_DOMAIN=billsplit.myhost.com npx expo prebuild   (or eas build)
 *
 * The same domain must serve /.well-known/apple-app-site-association and
 * /.well-known/assetlinks.json (see public/.well-known and README).
 */
module.exports = ({ config }) => {
  const domain = process.env.APP_LINK_DOMAIN;
  if (!domain) return config;
  return {
    ...config,
    ios: { ...config.ios, associatedDomains: [`applinks:${domain}`] },
    android: {
      ...config.android,
      intentFilters: [
        {
          action: 'VIEW',
          autoVerify: true,
          data: [{ scheme: 'https', host: domain, pathPrefix: '/invite' }],
          category: ['BROWSABLE', 'DEFAULT'],
        },
      ],
    },
  };
};
