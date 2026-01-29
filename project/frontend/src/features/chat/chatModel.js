// Chat state structure
// {
//   sessionId: string,
//   messages: Message[],
//   isLoading: boolean,
//   error: string | null
// }

export function createInitialChatState() {
  return {
    sessionId: null,
    messages: [],
    isLoading: false,
    error: null,
  };
}
