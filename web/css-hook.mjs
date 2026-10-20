// Load hooks that let the real component graph be imported in Node: Vite handles
// the CSS side-effect import in the browser, so here it becomes an empty module.
export async function resolve(specifier, context, nextResolve) {
  if (specifier.endsWith('.css')) {
    return { url: 'data:text/javascript,export default {}', shortCircuit: true }
  }
  return nextResolve(specifier, context)
}
