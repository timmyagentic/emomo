const { getDefaultConfig } = require('expo/metro-config');

const config = getDefaultConfig(__dirname);
const defaultResolveRequest = config.resolver.resolveRequest;

config.resolver.resolveRequest = (context, moduleName, platform) => {
  if (moduleName.startsWith('.') && moduleName.endsWith('.js')) {
    const withoutJs = moduleName.slice(0, -3);
    for (const extension of ['.ts', '.tsx']) {
      try {
        return context.resolveRequest(context, `${withoutJs}${extension}`, platform);
      } catch {
        // Fall through to the next extension and then Metro's default resolver.
      }
    }
  }

  if (defaultResolveRequest) {
    return defaultResolveRequest(context, moduleName, platform);
  }
  return context.resolveRequest(context, moduleName, platform);
};

module.exports = config;
