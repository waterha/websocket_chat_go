export function toggleConversationSelection(activeID, chatVisible, clickedID) {
  if (activeID === clickedID && chatVisible) return { activeID, chatVisible: false }
  return { activeID: clickedID, chatVisible: true }
}
