// postSignInPath decides where to land the user after every successful
// auth flow (login, signup, OAuth callback, magic-link, 2FA verify,
// invite accept).
//
// Scenario 39: passwordless users — those who created their account via
// magic-link or OAuth and have no PasswordHash yet — get a one-time
// promo to set a password before reaching the workspace. The promo is
// skippable. Users who already have a password skip straight to the
// workspace picker (the picker handles single-vs-multi-tenant routing).
//
// Caller does:
//   navigate(postSignInPath(data.user), { replace: true })
export function postSignInPath(user) {
  if (user && user.has_password === false) {
    return '/auth/set-password-promo'
  }
  return '/auth/pick-workspace'
}
