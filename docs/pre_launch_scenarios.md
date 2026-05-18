# Pre-launch user scenarios — пункты 1 (auth) + 2 (PIM)

> Каталог всех end-to-end пользовательских сценариев для прод-готового продукта.
> Формат: **что делает пользователь** + **что под капотом** (БД / email / UI / state / edge case).
> Помечай: ✅ ок / ⚠ полу-ок / ❌ не работает / ❓ не проверял.
> 📗 — нужно твоё решение (продуктовый вопрос).
> Сценарии 3-4 (V5 чат / композиции) НЕ включены — отдельный документ.
>
> **Для контекста (не структура):** есть два общих сценария попадания юзера к нам — через Shopify App Store (sec 7-10) и через нашу прямую регистрацию (sec 1-6) с подключением каталога позже. Это просто чтобы держать в голове разделение, под капотом будем разбирать дальше.

---

## 1. Регистрация — email + пароль

1. Я как новый пользователь могу зарегистрироваться через форму signup (email + пароль + companyName); в БД создаётся row в `admin_users` с bcrypt-хешем пароля, в `catalog.tenants` создаётся новый тенант со slug из companyName, в `admin.user_tenants` добавляется membership с ролью `owner`, мне выдаётся пара access+refresh токенов (TTL 15м / 30д) и я переадресуюсь в админку. ❓

2. Я как пользователь не могу зарегистрироваться с уже занятым email — backend возвращает `ErrEmailExists`, frontend показывает «email уже занят». ❓

3. Я как пользователь не могу зарегистрироваться с паролем короче 6 символов — frontend валидирует до отправки, backend дополнительно отклоняет. ❓

4. Я как пользователь не могу зарегистрироваться с пустым email / пустым паролем / пустым companyName — все три обязательны. ❓

5. Я как пользователь регистрируюсь с companyName "!!!" (мусор) — slug fallback'ается в `store`. ❓

6. Я как пользователь регистрируюсь с companyName "Stone & Steel" — slug = `stone-steel` (амперсанд нормализуется). ❓

7. Я как пользователь регистрируюсь с email в верхнем регистре `USER@X.com` — в БД хранится lowercased; при следующем login регистро-нечувствительно. ❓

8. Я как пользователь зарегистрировался — в email НЕ приходит magic-link / verify (email_verify backend готов, frontend не подключён). ⚠ 📗 *(подтверждение email обязательно или опционально?)*

## 2. Вход — email + пароль

9. Я как существующий пользователь могу войти через форму login (email + пароль); сверяется bcrypt, выдаётся session pair, в `admin.sessions` добавляется row с user_agent + ip; `last_login_at` стампится. ❓

10. Я как пользователь ввожу несуществующий email — backend возвращает `ErrInvalidCredentials` (не сообщает что email не найден — anti-enumeration). ❓

11. Я как пользователь ввожу правильный email + неправильный пароль — `ErrInvalidCredentials`. ❓

12. Я как пользователь ввожу пустой email или пустой пароль — отклоняется до проверки в БД. ❓

13. Я как пользователь с включённой TOTP вхожу через email+пароль — НЕ получаю access токен, получаю `pre_2fa_token` (TTL 5м) и `requires_2fa: true`. ❓

14. Я как пользователь нажимаю «remember me» — refresh token хранится в localStorage 30 дней. ❓ (или зависит от реализации фронта)

15. Я как залогиненный пользователь нажимаю «sign out» — refresh token revoke'ается, browser cookies очищаются, редирект на /signin. ❓

15a. На странице /signin над формой показывается «Last time you signed in with Google» (если такой метод был за последние 30 дней) — UX-подсказка, ускоряет повторный вход. ❌ *(не реализовано, твоя идея из коммента #25 — добавляем)* 📗 *(30 дней ОК?)*

## 3. Сессии / refresh / breach

16. Я как залогиненный пользователь автоматически обновляю access token через refresh — фронт сам делает это в фоне, старый refresh row revoke'ается, новый создаётся (rotation). ❓ *(автоматом, твой #16)*

17. Я как пользователь использую один и тот же refresh token дважды — backend интерпретирует это как breach: ВСЕ сессии этого user revoke'аются, я получаю `ErrInvalidCredentials` и должен входить заново. Это защита от украденного refresh. ❓

18. Я как пользователь использую refresh после истечения TTL (30+ дней) — отклоняется. ❓

19. Я как пользователь могу посмотреть список своих активных сессий (Settings → Sessions): показываются **браузер + ОС (распарсенный user-agent), примерная локация (geo по IP, например через MaxMind), время создания, текущее устройство помечено «это текущая сессия»**. ❌ *(сейчас в коде только сырой user-agent + ip, нужно добавить parsing + geo, твой #19)*

20. Я как пользователь могу отозвать конкретную сессию из списка (например, забыл выйти на чужом компе) — только владелец может отозвать свою сессию. ❓

21. Я как пользователь нажимаю «sign out on all devices» — все мои refresh row помечаются revoked, все мои устройства разлогиниваются. ❓

22. Я как пользователь использую refresh token который был revoke'нут — `ErrInvalidCredentials`. ❓

## 4. Google OAuth

23. Я как новый пользователь нажимаю «Continue with Google»; редирект на Google consent → callback → backend создаёт state (10 мин TTL) → потом state'ом consume'ит, создаётся новый тенант + admin_user с привязанным google_sub, выдаётся session pair. ❓

24. Я как существующий пользователь (зарегистрированный через email+пароль) нажимаю «Continue with Google» с тем же email — backend находит меня по email (step 2 cascade), линкует google_sub к существующему user, отдаёт `LinkedFromEmail: <email>` чтобы фронт показал баннер «Welcome back, we connected Google». ❓

25. Я как существующий пользователь который уже логинился через Google ранее (step 1 cascade) — фронт НЕ показывает «linked» баннер, просто логин. ❓ *(see also 15a для подсказки last method)*

26. Я как пользователь начинаю Google flow, но возвращаюсь с истёкшим state (>10 мин) — фронт показывает экран **«Time to sign in has expired, please try again»** с кнопкой «Back to sign in», через 3 сек авто-редирект на /signin. ❌ *(сейчас безликий AuthErrorPage, нужен дружелюбный текст + flow возврата, твой #26)*

27. Я как пользователь возвращаюсь с code/state который уже был использован — отказ. **По-человечески:** ссылка/код одноразовые; если я кликнул «назад» в браузере и опять отправил callback, мы это распознаём как replay и не даём войти повторно. Экран «This link is single-use, please try signing in again». ❌ *(твой #27 — расшифровка + дружелюбный экран)*

28. Я как пользователь возвращаюсь с state но kind=`telegram_login` (попытка cross-kind) — отклоняется. ❓

29. Я как пользователь отклоняю consent на стороне Google — фронт показывает экран **«You didn't allow Google access. Want to try again or sign in another way?»** с двумя кнопками: «Try Google again» и «Other methods». ❌ *(сейчас безликий google_rejected, нужен дружелюбный flow возврата, твой #29)*

30. Я как пользователь регистрируюсь через Google с email `Owner@MyShop.com` — backend нормализует к lowercased; следующий signin узнаёт user. ❓

31. Я как пользователь регистрируюсь через Google без `name` в профиле — companyName fallback к email-prefix; пользователь сможет переименовать workspace в Settings потом. ❓ ✅ *(твой #31 — флоу допускает, нам неважно что там за компания)*

## 5. Telegram OIDC

32. Я как новый пользователь нажимаю «Continue with Telegram» → redirect к Telegram OIDC → callback → создаётся state + новый тенант + admin_user с привязанным telegram_id, выдаётся session pair. ❓

33. Я как существующий пользователь (email+пароль или Google) с тем же email нажимаю Telegram — backend находит меня по email, линкует telegram_id, показывает linked-баннер. ❓

34. Я как существующий Telegram-пользователь (был раньше) — fast path step 1, без баннера. ❓

35. Я как пользователь возвращаюсь с истёкшим/неправильным state — отклоняется (см. 26 — нужен дружелюбный экран, аналогично Google). ❌

36. Я как пользователь возвращаюсь с handler URL содержащим `#tgAuthResult` (а не настоящий OIDC code) — backend имеет специальную обработку этого формата (commit 109378a). ❓

37. Я как пользователь имею аккаунт через Telegram legacy widget (старая интеграция) — fallback handler работает, новых OIDC redirect'ов система не делает. ⚠ (legacy mode)

## 6. Magic link (без Shopify)

38. Я как пользователь забыл пароль / хочу passwordless вход — на странице /signin кликаю «Forgot password / Sign in by email», ввожу email → backend создаёт challenge с code_hash, отправляет email через Resend с ссылкой `/auth/magic?code=<code>`. ⚠ *(backend готов, фронт за фичефлагом — нужно открыть, твой #38)*

39. Я как пользователь кликаю по магик-линку → frontend POST'ит code → backend consume'ит challenge (помечает consumed_at), выдаёт session pair, **редирект на `/auth/magic-success` с формой «Set a new password now» + nudge «Recommended to set a new password right now since you came in via email link»**. После save → Settings. ❌ *(сейчас редирект сразу в /catalog, твой #39 — нужно добавить промежуточную страницу с принуждением к смене пароля)* 📗 *(можно ли скипнуть и продолжить без смены или обязательно?)*

40. Я как пользователь кликаю по уже использованному магик-линку — экран **«This link is single-use, you've already used it»** + форма «Request a new one» (поле email + кнопка Send) + ссылка «Back to sign in». ❌ *(твой #40)*

41. Я как пользователь кликаю по истёкшему магик-линку (>24h) — экран **«This link expired (links live for 24 hours)»** + та же форма «Request a new one» + ссылка «Back to sign in». ❌ *(твой #41)*

42. Я как пользователь получаю magic-link на email который НЕ зарегистрирован в системе — challenge не создаётся, email не уходит (молча no-op чтобы не утечь email-enumeration). ❓ ✅ *(твой #42 — да гут)*

43. Я как пользователь запрашиваю magic-link когда mailer (Resend) недоступен — challenge всё равно создаётся (чтобы повторная попытка отправки работала), но email не уходит, в логах warning. Эти логи видны куратору в его глобальном дашборде (см. секция 30). ❓ *(твой #43 — добавил секцию 30)*

## 7. Shopify install — happy path (с owner email)

44. Я как merchant в Shopify App Store нажимаю «Add app» → Shopify редиректит на наш `/shopify/app/entry` с HMAC; backend verify'ит HMAC, делает dup-check (если уже installed для этого shop'а — reinstall path), auto-provision tenant. ❓

45. После entry я как merchant прохожу через OAuth → callback на наш `/shopify/install/complete` → backend exchange'ит code на access_token, шифрует его (secretbox) и пишет в `catalog.shopify_integrations` с `connected_at=NOW()`. ❓

46. После integration created backend запускает ShopifyIngester (через runInitialIngest) → bulk pull продуктов → пишет в `catalog.inbox_items` с source_kind=shopify, external_id=GID, raw=JSONB полного payload'а, payload_hash=sha256. ❓

47. В action_log пишется `connect` (ok), потом `inbox_pull` (ok) с counts (inserted/updated/unchanged). ❓ 📗 *(после `inbox_pull` discovery запускается сразу или ждёт первого apply / Sync now? сейчас — ждёт)*

48. Параллельно backend дёргает Shopify Admin API за shop info → находит `customerEmail` (owner email) → запускает ProvisionShopOwner ASYNC. ❓

49. ProvisionShopOwner: если user с этим email НЕ существует → создаётся новый admin_user (без пароля, passwordless), ему grant'ится owner-membership на этот tenant, ему отсылается magic-link через Resend. ❓

50. Я как merchant получаю magic-link на свой Shopify owner email → клик → /auth/magic → consume → попадаю в админку, вижу подключённый магазин. ❓

51. Если ProvisionShopOwner: `memberships.Add` падает для свежесозданного user (race-condition) → backend НЕ отсылает email (защита: пустой пользователь без memberships не должен получать ссылку). User-row остаётся для ручного восстановления. ❓

## 8. Shopify install — owner email уже есть в системе (НУЖЕН CONSENT)

52. ❌ Сейчас в коде: существующий user re-use'ится, ему добавляется membership на новый tenant, отсылается magic-link. Это **уязвимость**: я могу подставить чужой email в Shopify и попасть в чужую админку. **Надо переделать на consent flow** (твой #52).

53. Должно быть так: создаётся `pending_claim` challenge → существующему юзеру летит письмо «Кто-то поставил Shopify app `foo.myshopify.com` под email который привязан к твоему аккаунту. Подтвердить / отклонить». ❌ *(не реализовано)*

54. Юзер кликает «подтвердить» → membership добавляется к существующему юзеру, в picker'е появляется новый workspace; magic-link не нужен (он залогинится обычным способом). ❌

55. Юзер кликает «отклонить» → tenant помечается отказанным, через cleanup-job удаляется. Никто туда залогиниться не может. ❌

56. Юзер игнорирует письмо — tenant в подвешенном состоянии. В Settings → Memberships → Pending существующего юзера показывается badge «1 магазин ждёт подтверждения». 📗 *(такой badge нужен или достаточно email-only?)*

## 9. Shopify install — НЕТ owner email (pending_link path)

57. Я как merchant ставлю Shopify-приложение, но в shop info `customerEmail` пустой (privacy setting или dev-store) → ProvisionShopOwner не запускается, backend issue'ит `shop_pending_link` challenge с tenant_id в meta. ❓

58. Backend редиректит меня после OAuth callback на `/auth/install-complete?pending_link=<token>&shop=<domain>`. ❓

59. Я как merchant на `/auth/install-complete` вижу страницу «We've installed your Shopify app. Sign in to start» с кнопками Google/Telegram/Email; frontend сохраняет `pending_link` в sessionStorage. ❓

60. Я как merchant выбираю любой метод входа (например Google) → после успешного signin frontend ConsumePendingLink → backend линкует мой user_id с tenant_id из challenge meta, добавляет membership. ❓

61. Я как merchant отказываюсь входить и закрываю вкладку — tenant остаётся orphan, через cleanup-tenant-stale job (если запущен) удаляется. ❓ (cron не запущен — известный gap) ⚠

## 10. Shopify install — reinstall / uninstall

62. Я как merchant удалил приложение в Shopify → Shopify шлёт webhook `app/uninstalled` → handler verify'ит HMAC, в БД `shopify_integrations.disconnected_at=NOW()`, в action_log пишется `disconnect`. ❓

63. Я как merchant переустанавливаю приложение → entry handler видит существующий integration в БД (disconnected_at не NULL), reinstall path: создаёт новый access_token, очищает disconnected_at, ingester запускается заново. ❓

64. Если webhook `app/uninstalled` пришёл с невалидным HMAC — отклоняется 401, integration не трогается. ❓

65. Если webhook `app/uninstalled` пришёл во время фонового runInitialIngest (race) — контекст должен НЕ отменяться (commit историческая регрессия: 8ddaaa5 и ранее, fix: использовать bgCtx, не r.Context()). ❓

## 11. Workspace picker (multi-tenant user)

66. Я как пользователь с членством в нескольких workspace после signin вижу picker «Select workspace» со списком моих tenants + ролей. ❓

67. Я как пользователь выбираю один из workspace → backend выдаёт новую session pair через IssueForTenant с `tid` claim = выбранному tenant и `role` = моей role в нём. ❓

68. Я как пользователь могу переключаться между workspaces из UI (Settings → Switch workspace) без полного logout. ❓

69. Я как пользователь имею только один workspace — picker НЕ показывается, сразу попадаю в админку. ❓

70. Я как пользователь попадаю в picker, но мой только workspace soft-deleted (orphan cleanup сработал) — backend отдаёт «no active workspace», UI показывает «Contact support». ❓

## 12. Приглашения (Invitations)

71. Я как owner/admin workspace'а могу пригласить кого-то по email через UI (Settings → Members → Invite); backend создаёт row в `admin.invitations` с TTL 7 дней, отсылает email с ссылкой `/auth/accept-invite?token=<token>`. ❓

72. Я как owner вижу rate-limit «invite quota exceeded» если выслал больше 20 приглашений за 24h. ❓

73. Я как owner приглашаю с пустым email / невалидным role (`role` не из owner|admin|member|viewer) — отклоняется. ❓

74. Я как owner приглашаю кого-то с email в верхнем регистре `Guest@X.COM` — в БД хранится lowercased. ❓

75. Я как приглашённый пользователь, НЕ залогинен, кликаю ссылку → попадаю на /auth/accept-invite → preview показывает «Workspace Foo invited you as admin, expires in N days». ❓

76. Я как приглашённый пользователь, аккаунта в системе НЕТ — ввожу пароль на форме accept → backend создаёт новый admin_user с password_hash, добавляет membership на tenant из приглашения, помечает invitation accepted_at, выдаёт session pair. ❓

77. Я как приглашённый пользователь, у меня УЖЕ есть аккаунт но я не залогинен → backend reuse'ит мой user, добавляет membership, выдаёт session pair (НЕ переписывает пароль из формы). ❓

78. Я как приглашённый пользователь, уже залогинен — frontend не показывает форму пароля, backend Just-Add-Membership, возвращает existing user без token-pair. UI редиректит в новый workspace. ❓

79. Я как приглашённый пользователь кликаю по уже use'нутому invitation token — «invitation already accepted». ❓

80. Я как приглашённый пользователь кликаю по истёкшему (>7d) token — «invitation expired». ❓

81. Я как приглашённый пользователь использую неизвестный token — `ErrInvalidCredentials`. ❓

82. Если mailer недоступен при Create — invitation row создаётся, но email не уходит. ❌ **Известный gap** — invitee никогда не узнает что был приглашён. Нужен retry-job или UI «отправить ещё раз». ⚠ 📗

## 13. 2FA — TOTP

83. Я как залогиненный пользователь могу включить TOTP (Settings → Security → 2FA): backend генерит секрет (`totp.NewSecret`), шифрует через secretbox.Box, сохраняет в `admin_users.totp_secret_encrypted` с `totp_enabled_at=NULL`, возвращает otpauth-URL для QR-кода. ❓

84. Я как пользователь сканирую QR кодом в Google Authenticator / 1Password / Authy → вижу 6-значный код. ❓

85. Я как пользователь ввожу первый код для подтверждения → backend верифицирует через `totp.Verify`, стампит `totp_enabled_at=NOW()`. Если код неправильный — `ErrInvalidCredentials`, секрет НЕ удаляется (можно retry). ❓

86. Я как пользователь с включённой TOTP пытаюсь войти через email+пароль — Login возвращает `Requires2FA: true` + pre_2fa_token, фронт показывает форму ввода кода. ❓

87. Я как пользователь ввожу TOTP код на форме 2FA → backend VerifyTOTP → выдаёт session pair, `last_login_at` стампится. ❓

88. Я как пользователь ввожу неправильный TOTP код — `ErrInvalidCredentials`, могу retry. ❓

89. Я как пользователь могу выключить TOTP (Settings → Security → Disable 2FA) — **перед выключением фронт требует повторного ввода пароля + TOTP-кода** (re-auth, защита от ZB-takeover). ❌ *(сейчас в коде проверки нет)* 📗

## 14. 2FA — Email code

90. Я как пользователь с включённой email-2FA вхожу через email+пароль → backend SendEmailCode → отсылает 6-значный код через Resend, challenge с TTL 15 минут. ❓

91. Я как пользователь ввожу код из email на форме 2FA → backend VerifyEmailCode → выдаёт session pair, challenge помечается consumed. ❓

92. Я как пользователь ввожу неправильный код — `ErrInvalidCredentials`. ❓

93. Я как пользователь ввожу свой код но `userID` из pre_2fa_token не совпадает — `ErrInvalidCredentials` (anti-replay). ❓

94. Я как пользователь не успеваю ввести код за 15 минут — challenge истёк, нужен новый Send. ❓

95. Если mailer недоступен при SendEmailCode — пользователь не сможет войти. UI показывает «Не получилось отправить код, попробуйте позже или войдите через TOTP / другой способ». ❓

96. Я как пользователь имею TOTP+Email 2FA ОБА включёнными — login возвращает `Requires2FA: true` + `Has2FAEmail: true`, фронт показывает выбор какой ввести. ❓ 📗 *(оба сразу разрешены или ограничить одним?)*

## 15. Email verify / password reset

97. Я как пользователь могу запросить «forgot password» по email на /auth/forgot — backend issue'ит password_reset challenge, отсылает ссылку. ⚠ *(Backend готов, frontend за фичефлагом. Может быть избыточно при наличии magic-link (см. 38) — оба пути ведут к смене пароля)* 📗 *(нужен отдельный forgot-password или magic-link достаточно?)*

98. Я как пользователь кликаю по reset-ссылке → форма «new password» → POST → backend validate'ит strength + Consume + UpdatePasswordHash. ❓

99. Я как пользователь кликаю по reset-ссылке которая уже use'нута/истекла — отклоняется (нужны экраны как в 40/41). ❌

100. Я как пользователь могу запросить email verify (Settings → Verify email) — backend issue'ит email_verify challenge. ⚠ *(Backend готов, UI кнопки нет)*

101. Я как пользователь кликаю по verify-ссылке → backend Consume → `admin_users.email_verified_at=NOW()`. ❓

102. Я как пользователь могу resend verify-ссылку с rate-limit. ⚠ *(Backend готов, UI нет)*

## 16. PIM — Inbox ingest (Shopify bulk)

103. Я как merchant подключаю Shopify → ShopifyIngester делает bulk operations query → получает JSONL → парсит → каждый product = 1 inbox_item с source_kind='shopify', external_id=Shopify GID, raw=полный JSONB, payload_hash=sha256(raw). ❓

104. Я как merchant пере-подключаю тот же Shopify → bulk pull снова → `Upsert` определяет: если payload_hash совпадает (товар не менялся) — `changed=false`, обновляет только `fetched_at`; если hash отличается — `changed=true`, переписывает `raw` и **сбрасывает applied_at=NULL** (apply подхватит на следующем проходе). ❓

105. Я как merchant имею 1000 SKU в Shopify — bulk operation gracefully handle'ит большой объём (Shopify streams JSONL, ingester батчит). ❓

106. Я как merchant имею товар с не-ASCII символами в title (русский, эмодзи, китайский) — `raw` JSONB корректно хранит UTF-8, никаких \u escape. ❓

107. action_log пишет `inbox_pull` с payload `{source, total, inserted, updated, unchanged, errors}`, status=ok (или warning если errors>0). ❓

## 17. PIM — Inbox ingest (CSV / Sheets / manual)

108. Я как merchant загружаю CSV через `/admin/api/catalog/import/csv` (если UI есть) — каждая row = 1 inbox_item, external_id = SKU из CSV, или если пусто — hash от значимых полей. **Endpoint deferred per Shopify-only scope cut.** ❓ deferred

109. Я как merchant делаю manual product entry через UI — `InboxUseCase.WriteManual` с external_id=UUID, source_kind='manual'. **Frontend не подключён.** ❓ deferred

110. Я как merchant через CSV: тот же SKU дважды в файле → второй upsert переписывает первый (idempotent по `(tenant, source, external_id)`). ❓ deferred

## 18. PIM — Discovery v2 (first_install)

111. Когда inbox_items наполнились впервые для нового tenant → apply_v2 видит artifact=NULL → cascade'ится в `discovery.Discover(trigger='first_install')`. ❓

112. Discovery agent запускается с Sonnet 4.6, $5 budget, 30 turns max, 10 min wallclock, prompt-cached system block. ❓

113. Agent делает 5-15 tool calls (count_total, list_fields, sample_values, count_by, field_stats, peek_full_rows), стоит ~$0.04-0.06 для 50 SKU. ❓

114. Agent коммитит artifact через `commit_artifact` tool → MappingArtifactV3 с branches[] (по одному per vertical) + classify_rules → сохраняется в `catalog.tenant_catalog_schema.mapping_artifact`. ❓

115. action_log пишет `discovery_start` (ok) и `discovery_done` (ok+committed:true). agent_runs хранит full timeline с tokens, cost, tool_calls JSONB. ❓

116. Если у tenant 10 SKU косметики — agent коммитит artifact с одним branch=cosmetics. ❓

117. Если у tenant 10 cosmetics + 10 electronics — agent коммитит artifact с двумя branches и optionally classify_rules для disambiguation. ❓

118. Если agent доходит до 90% от $5 budget — на следующем turn'е дополнительный nudge «force commit». ❓

119. Если agent end_turn'ит без commit — до 3 nudge'ей «вы должны вызвать commit_artifact». Потом — fail. ❓

120. Если budget exhausted без commit — artifact_run помечается `budget_exhausted`, действующий artifact (если был) НЕ перезаписывается. ❓

## 19. PIM — Discovery v2 (mapping_miss / unknown_vertical)

121. Когда apply_v2 натыкается на rule которая не может транформ'ить значение → wraps в mappingMissErr → action_log пишет `mapping_miss`, триггерит `discovery.Discover(trigger='mapping_miss')` с payload {inbox_item_id, offending_from, offending_to, reason}. ❓

122. Narrow discovery_v2 (mapping_miss) фокусируется на конкретном поле — system prompt инструктирует «не re-discover весь каталог», обычно 3-5 tool calls, $0.01. ❓

123. После narrow discovery apply_v2 re-fetch'ит artifact и продолжает loop. ❓

124. Если за один apply-run сработало больше 3 mapping_miss подряд — дальнейшие НЕ триггерят discovery (защита от storm'а). ❓

125. Если row классифицируется в vertical для которого нет branch'a в artifact (например свежий tenant добавил товар категории furniture, а артефакт был cosmetics-only) → wraps в mapping_miss с trigger=unknown_vertical → narrow discovery добавляет недостающую branch. ❓

126. action_log за весь apply-run суммирует counts (applied / errors / mapping_misses / skipped) в одной записи `apply` со status'ом (ok / warning / error). ❓

## 20. PIM — Apply v2 (create master)

127. Когда inbox_item с уникальным SKU и нет такого мастера в БД — `MatchOrCreateMaster` INSERT'ит новый row в `catalog.master_products` (Tier 1: name, brand, vertical, sku, gtin, normalized_match_key, owner_tenant_id, tier3 JSONB). ❓

128. Vertical берётся НЕ из rule (master.vertical), а из per-row классификации (`ClassifyVertical` cascade: alias → classify_rule → unknown). Это перебивает любой artifact rule на master.vertical. ❓

129. Если row классифицирован как `cosmetics` и есть `cosmetics.*` rules — создаётся row в `catalog.master_cosmetics` (Tier 2 типизированная: skin_type[], concern[], key_ingredients[], product_form, texture, volume_ml, spf, и т.д.). ❓

130. Если row классифицирован как `electronics`/`furniture`/`unknown` — master_cosmetics НЕ создаётся; per-vertical поля (cpu/ram_gb/material) идут в `master_products.tier3` JSONB. ❓

131. Если agent эмитит `cosmetics.spf` rule в `unknown` branch — apply_v2 forgiving fallback: пишет в tier3.spf с warning в логе (НЕ mapping_miss). ❓

132. Если agent эмитит `tier3.cpu` для electronics branch — нормально, пишется в tier3. ❓

133. Если agent эмитит rule с unknown prefix (например `services.duration_min`) и `vertical.column` форму — apply_v2 reroute'ит в tier3 с warning (forgiving fallback). ❓

134. GTIN/barcode/ean/upc rules → все мапятся в `master_products.gtin`, очищаются от не-цифр. ❓

135. normalized_match_key computed via `NormalizeMatchKey(brand, name)` — lowercased, не-alnum stripped, multi-space collapsed. ❓

## 21. PIM — Apply v2 (bind cascade + master immutability)

136. Когда inbox_item с тем же SKU как существующий мастер — `MatchOrCreateMaster` возвращает (existing_master_id, wasCreated=false). Master row **НЕ перезаписывается**. ❓

137. Когда SKU не совпадает, но GTIN совпадает с существующим мастером — bind по GTIN (stage 2 каскада). Master immutable. ❓

138. Когда ни SKU ни GTIN не совпадают, но normalized_match_key совпадает — bind по match_key (stage 3). Master immutable. ❓

139. Когда ВСЕ три миссы — INSERT нового master. ❓

140. На bind master_products **не апдейтится**, master_cosmetics **не апдейтится**, tier3 **не апдейтится**. Записывается ТОЛЬКО listing. ❓

141. Listing (catalog.products) ВСЕГДА upsert'ится по (tenant_id, source_system, source_id) — это per-tenant overlay. Поля: price_cents, currency, stock_quantity, custom_title, master_product_id, source_id (Shopify GID). ❓

142. Per-tenant marketing customization (теги, картинки, видео) **deferred** — колонка `listing.tenant_overrides` JSONB зарезервирована, но writer не подключён. ⚠

## 22. PIM — Apply v2 (synthetic SKU / junk rejection)

143. Если row имеет name но не имеет SKU — apply_v2 синтезирует `auto-<source>-<external_id>` (например `auto-shopify-gid://shopify/Product/123`). Bind cascade всё равно пробуется по GTIN и normalized_match_key до синтеза. ❓

144. Если row не имеет ни name ни SKU — **junk row**, raises mapping_miss с reason "row produced neither name nor sku", не пишется ни мастер ни listing. ❓

145. Если row имеет name из спам-слов / random unicode но всё-таки заходит в каталог — мы пишем мастер. Никакой content moderation. ❓ (известный gap, не блокер) 📗 *(нужна content moderation или ок?)*

## 23. PIM — Match cascade contract

146. `MatchOrCreateMaster` атомарен на уровне адаптера: cascade + INSERT в одной транзакции с `ON CONFLICT DO NOTHING` для race-recovery. ❓

147. Если две параллельные apply-сессии пытаются INSERT мастер с одним SKU — выигрывает первая, вторая делает SELECT и возвращает существующий id. Никаких duplicate row. ❓

148. SKU comparison case-sensitive (Shopify normalizes сам). GTIN digit-only. normalized_match_key уже lowercased. ❓

## 24. PIM — Webhook updates (in-window / out-of-window)

149. Я как merchant изменяю цену товара в Shopify → Shopify шлёт webhook `products/update` → handler verify'ит HMAC, парсит payload, передаёт в `UpdateOrchestrator.OnWebhook(ev)`. ❓

150. OnWebhook ВСЕГДА пишет `webhook_received` в action_log + ВСЕГДА upsert'ит inbox_item (даже если apply не запустится). ❓

151. Если с последнего apply прошло <24 часов — OnWebhook возвращает `absorbed_only`, apply НЕ запускается. ❓

152. Если с последнего apply прошло >24 часов — OnWebhook запускает apply_v2 → listing.price_cents обновляется до новой цены. ❓

153. Я как merchant изменяю 50 товаров подряд → 50 webhook'ов → 50 inbox upsert'ов + 50 webhook_received log entries, apply НЕ запускается per-event (rate-limit). ❓

154. Через 24 часа после последнего apply следующий webhook автоматически запустит apply (или manual sync). ❓

## 25. PIM — Webhook delete

155. Я как merchant удаляю товар в Shopify → webhook `products/delete` → handler verify'ит HMAC → вызывает `inboxIngester.SoftDeleteListing(tenant, source_system, source_id)`. ❓

156. SoftDeleteListing стампит `catalog.products.deleted_at=NOW()` для матчинг listing'а. `master_products` row НЕ трогается (другие тенанты могут на него ссылаться). ❓

157. inbox_item для удалённого товара НЕ удаляется автоматически — остаётся как исторический snapshot. ❓ (deferred cleanup decision)

158. V5 chat при поиске пропускает rows с `deleted_at IS NOT NULL`. ❓

159. Delete webhook идемпотентный — повторный вызов на уже-удалённый listing — no-op. ❓

## 26. PIM — Manual sync (Sync now button)

160. Я как куратор/admin нажимаю «Sync now» в curator UI для конкретного tenant → curator POST'ит к admin backend `/admin/api/catalog/v2/sync-now/{tenant_id}` через X-Internal-Key middleware. ❓

161. ManualSync **bypass'ит** 24h rate-limit, всегда запускает apply_v2. ❓

162. action_log пишет `manual_sync` (ok) + потом `apply` с результатами. ❓

163. Если нет artifact'а — ManualSync каскадно запустит discovery_v2 first_install. ❓

164. Если ManualSync запущен параллельно для того же tenant — обычно одна выиграет, вторая увидит уже apply'нутые items (idempotent). ❓

## 27. PIM — Curator UI (per-tenant view)

165. Я как куратор открываю TenantDetailPage для tenant'а — вижу 5 табов: Catalog, Inbox, Mapping, Action Log, Agent Runs. ❓

166. Catalog tab показывает все listings tenant'а с master-link badge (MASTER если bound, OWNED если этот тенант owns master). ❓

167. Inbox tab показывает все inbox_items (raw payload, JSONB preview, applied_at status). Можно открыть detail modal с full payload. ❓

168. Mapping tab показывает текущий MappingArtifactV3: branches[] + classify_rules + notes + built_at. JSON view. ❓

169. Action Log tab показывает timeline всех action_log записей: inbox_pull, discovery_start, discovery_done, apply, mapping_miss, webhook_received, manual_sync, connect, disconnect. ❓

170. Agent Runs tab показывает список discovery_v2 запусков с trigger, status, cost_usd, tokens. Detail modal — tools_called JSONB полностью (waterfall туллов). ❓

171. Куратор НЕ может edit'ить master через UI (deferred). ❓ deferred 📗 *(нужно до запуска?)*

172. Куратор НЕ может edit'ить mapping_artifact вручную (deferred). ❓ deferred 📗

173. Куратор НЕ может добавить vertical alias (deferred). ❓ deferred 📗

## 28. PIM — Disconnect / cleanup

174. Я как merchant disconnect'ю Shopify через `app/uninstalled` webhook → `shopify_integrations.disconnected_at` стампится. ❓

175. После disconnect inbox_items, master_products, catalog.products **НЕ удаляются** автоматически — данные сохраняются на случай reinstall. ❓

176. Я как куратор могу soft-delete tenant полностью через cleanup-tenant-stale CLI → `catalog.tenants.deleted_at` стампится, FK cascade удаляет inbox/products/integrations. ❓ (cron не запущен)

177. Disconnected tenant НЕ показывается в picker'е после signin. ❓

## 29. Edge cases / failure modes

178. Я как merchant имею Shopify shop с >10 000 SKU — bulk operation streaming должен gracefully handle'ить, не OOM'нуть. ❓

179. Я как merchant имею Shopify shop где `customerEmail` это alias / shared mailbox — magic-link приходит туда, могут несколько людей кликнуть; первый клик create'ит session, второй кликает по reused token — `ErrInvalidCredentials`. ❓

180. Я как пользователь без интернета пытаюсь signin — frontend показывает «Network error, try again». ❓

181. SMTP/Resend полностью down — magic-link/invitation/2FA email НЕ уходят, signin через email невозможен. Google/Telegram работают. **Risk**: новый Shopify install прервётся на этапе magic-link. 📗 *(нужен fallback — показывать ли code прямо на странице после OAuth callback?)*

182. Anthropic API down — discovery_v2 fail'ится, apply_v2 cascade fail'ится с понятной ошибкой. Существующие apply (с уже committed artifact) продолжают работать. ❓

183. Postgres down — все запросы fail'ятся, frontend показывает 500. ❓

184. Я как пользователь делаю replay-attack на refresh token — breach detection срабатывает (см. 17), вся сессия-семья revoke'нута. ❓

185. Я как admin/owner на одном workspace получаю Sentry/error alert если pipeline для моего tenant'а в state error >10 минут. **Sentry не настроен** — deferred. ❓ deferred

186. Я как merchant имею >100 mapping_miss за один apply — после 3-х narrow discovery срабатывает cap, остальные идут в apply.MappingMisses counter без триггера. action_log пишет `apply` со status=warning. ❓

187. Я как админ платформы (не реализовано) могу force-disconnect tenant. ❓ deferred

## 30. Куратор-дашборд (твой #43 — single pane of glass для всех юзеров)

188. Я как куратор открываю глобальный dashboard со всеми пользователями + tenant'ами + последними событиями. ❌ *(не реализовано)*

189. Dashboard показывает: список tenants (slug, последняя активность, статус integration), recent errors (failed signin, mailer down, discovery failed) за 24h, mailer health. ❌

190. Из dashboard'а могу провалиться в конкретный tenant — там 5 табов из 27. ❓

191. Могу искать пользователя по email → видеть все его memberships, последние сессии (см. 19 — браузер/OS/гео), метод последнего входа. ❌

192. Логи warning'ов от backend (mailer fail, discovery fail и т.д.) видны через dashboard — куратор может proactively реагировать. ❌

📗 *(минимальный набор полей на dashboard? я предложил выше — список тенантов, ошибки 24h, mailer health, поиск по email. Добавить что-то?)*

---

## Что закрыто кодом vs нет

**В коде есть полностью (можно проверять):** 1, 2, 9-13, 16-18, 20-25, 28, 30-34, 36, 39, 42, 44-51, 62-65, 66-70, 71-81, 83-88, 90-96, 98, 101, 103-107, 111-126, 127-148, 149-164, 165-170, 174-177, 179, 184, 186.

**Backend готов, frontend не подключён:** 8, 38, 97, 100, 102.

**❌ Нужно сделать до запуска:**
- 15a — UX «last login method» на /signin
- 19 — обогащение session list (parsed user-agent + geo + current session marker)
- 26, 29, 35, 99 — дружелюбные экраны ошибок auth с flow возврата
- 27 — экран «link single-use, try again»
- 39 — после magic-link → set-new-password nudge
- 40, 41 — экраны «used / expired» с формой re-request
- 52-55 — переделать Shopify auto-merge на consent flow (БЕЗОПАСНОСТЬ)
- 82 — retry для invite mailer fail
- 89 — re-auth перед выключением 2FA
- 188-192 — куратор-дашборд

**📗 Решения (твой ход):** 8, 15a, 39, 47, 56, 96, 97, 145, 171-173, 181.

**Deferred (явно отложено, не блокер запуска):** 56-61 fallback paths, 108-110 (CSV/manual), 142 (tenant_overrides writer), 145 (content moderation), 157 (inbox cleanup), 171-173 (curator edit UI), 176 (cleanup cron), 185 (Sentry), 187 (admin platform).

**Edge / infra (runtime, не код):** 178, 180-183.

---

## Следующий шаг

1. Ты проходишь дальше (был на 52 / Shopify), помечаешь ✅/⚠/❌ и закрываешь 📗
2. Я обновляю документ с финальными решениями
3. Список ❌ + закрытые 📗 = backlog работы до 20 мая
4. Сценарии чата — отдельный документ потом
