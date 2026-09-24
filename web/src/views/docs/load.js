// The docs prose, one chunk per language (content.<lang>.jsx), as loaders.
// Shared by the docs page, which renders them, and the app shell, which starts
// fetching the current language's chunk as soon as it knows the route is /docs
// instead of waiting for the docs page to render and ask.
export const CONTENT_LOADERS = Object.fromEntries(
  Object.entries(import.meta.glob('./content.*.jsx')).map(([path, load]) => [
    /content\.(\w+)\.jsx$/.exec(path)[1],
    load,
  ]),
);
