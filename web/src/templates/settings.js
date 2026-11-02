// A runtime template fragment; whitespace and bindings belong to the root view.
export default `\n    <DesktopSettings :logged-in="!!me" />
    <AppearanceSettings v-if="me && showAppearance" @close="showAppearance = false" />
`
