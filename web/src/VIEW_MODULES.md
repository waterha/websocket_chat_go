# View module map

```text
template.js (one root Vue runtime template; concatenation adds no separators)
├─ templates/settings.js + authentication.js
├─ templates/conversation-sidebar.js
├─ templates/chat-header.js + history-search.js
├─ templates/message-stream.js + message-selection.js + message-composer.js
├─ templates/conversation-details.js + thread-window.js
├─ templates/*-dialog.js + friend-dialogs.js (feature dialogs)
└─ templates/action-overlays.js + error-toast.js

style.css (ordered @import entry)
└─ styles/*.css (existing contiguous semantic sections, in original cascade order)
```

Template fragments remain plain strings in the root setup binding scope. The entry owns structural layout/chat wrappers and the empty-chat alternative; do not add separators, trim fragments, or move `v-if`/`v-else` siblings. All existing markup, indentation, refs and directives are retained.

Styles keep complete existing rule blocks, comments and media queries. The original sheet interleaves domains and later overrides: for example `rich-composer` precedes `composer-surface` and `composer-toolbar`; `group-panels` precedes `group-detail`; `login-action` follows `voice`. These separate stages are intentional. Keep the entry import order when editing; merging all rules of one domain together would change the cascade. Shared early rules remain in `foundation`, and shared final spacing/scrollbars remain in `interface-polish`.

Validation for this extraction compares the assembled template to its original runtime string and expands the CSS imports to compare the original stylesheet byte for byte. Runtime template/binding tests and Vite build cover the public entries without changing their consumers.
