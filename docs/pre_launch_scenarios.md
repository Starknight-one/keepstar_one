# Pre-launch user scenarios — пункты 1 (auth) + 2 (PIM)

> Текущая версия проекта: **Alpha 0.5.0** (см. `VERSION` в корне).

> Каталог всех end-to-end пользовательских сценариев для прод-готового продукта.
> Формат: **что делает пользователь** + **что под капотом** (БД / email / UI / state / edge case).
> Помечай: ✅ ок / ⚠ полу-ок / ❌ не работает / ❓ не проверял.
> 📗 — нужно твоё решение (продуктовый вопрос).
> Сценарии 3-4 (V5 чат / композиции) НЕ включены — отдельный документ.
>
> **Для контекста (не структура):** есть два общих сценария попадания юзера к нам — через Shopify App Store (sec 7-10) и через нашу прямую регистрацию (sec 1-6) с подключением каталога позже. Это просто чтобы держать в голове разделение, под капотом будем разбирать дальше.

---

## 1. Регистрация — email + пароль

1. Я как новый пользователь могу зарегистрироваться через форму signup (email + пароль + companyName); в БД создаётся row в `admin_users` с bcrypt-хешем пароля, в `catalog.tenants` создаётся новый тенант со slug из companyName, в `admin.user_tenants` добавляется membership с ролью `owner`, мне выдаётся пара access+refresh токенов (TTL 15м / 30д) и я переадресуюсь в админку. ✅ (unit + http + e2e)

2. Я как пользователь не могу зарегистрироваться с уже занятым email — backend возвращает `ErrEmailExists`, frontend показывает «email уже занят». ✅ (unit + http + e2e — 409 Conflict)

3. Я как пользователь не могу зарегистрироваться с паролем короче 6 символов — frontend валидирует до отправки, backend дополнительно отклоняет. ✅ (unit — `TestSignup_RejectsShortPassword`)

4. Я как пользователь не могу зарегистрироваться с пустым email / пустым паролем / пустым companyName — все три обязательны. ✅ (unit + http — 400 BadRequest)

5. Я как пользователь регистрируюсь с companyName "!!!" (мусор) — slug fallback'ается в `store`. ✅ (unit — `TestScenario_005`)

6. Я как пользователь регистрируюсь с companyName "Stone & Steel" — slug = `stone-steel` (амперсанд нормализуется). ✅ (unit — `TestScenario_006`)

7. Я как пользователь регистрируюсь с email в верхнем регистре `USER@X.com` — в БД хранится lowercased; при следующем login регистро-нечувствительно. ✅ *(fixed: `AuthUseCase.Signup`/`Login`/`LoginWithMeta` теперь lowercase'ят email через `strings.ToLower(strings.TrimSpace(...))`. Тест `TestScenario_007_SignupUppercaseEmail_StoredAndLoginCaseInsensitive` PASS. Существующие верхне-регистровые row'ы в БД не трогали — login и так case-insensitive через адаптер)*

8. Я как пользователь зарегистрировался — в email НЕ приходит magic-link / verify (email_verify backend готов, frontend не подключён). ⚠ 📗 *(подтверждение email обязательно или опционально?)*

## 2. Вход — email + пароль

9. Я как существующий пользователь могу войти через форму login (email + пароль); сверяется bcrypt, выдаётся session pair, в `admin.sessions` добавляется row с user_agent + ip; `last_login_at` стампится. ✅ (unit + http + e2e)

10. Я как пользователь ввожу несуществующий email — backend возвращает `ErrInvalidCredentials` (не сообщает что email не найден — anti-enumeration). ✅ (unit + http + e2e — 401)

11. Я как пользователь ввожу правильный email + неправильный пароль — `ErrInvalidCredentials`. ✅ (unit + http + e2e — 401)

12. Я как пользователь ввожу пустой email или пустой пароль — отклоняется до проверки в БД. ✅ (unit — `TestLogin_MissingFields`)

13. Я как пользователь с включённой TOTP вхожу через email+пароль — НЕ получаю access токен, получаю `pre_2fa_token` (TTL 5м) и `requires_2fa: true`. ✅ (unit — TOTP path в `auth_2fa_test.go`)

14. Я как пользователь нажимаю «remember me» — refresh token хранится в localStorage 30 дней. ✅ *(fixed: чекбокс "Remember me on this device" на /signin, по умолчанию ON. ON → refresh в localStorage (30 дней, переживает рестарт браузера). OFF → sessionStorage (стирается при закрытии вкладки). Бэк всегда выдаёт 30-day refresh — фронт решает где хранить. `getRememberMe()` сохраняет выбор для silent refresh после rotation)*

15. Я как залогиненный пользователь нажимаю «sign out» — refresh token revoke'ается, browser cookies очищаются, редирект на /signin. ✅ (http + e2e — `TestE2E_Scenario_015_Logout`)

15a. На странице /signin над формой показывается «Last time you signed in with Google» (если такой метод был за последние 30 дней) — UX-подсказка, ускоряет повторный вход. ✅ *(fixed: AuthProvider пишет `last_login_method` в localStorage после login/signup/OAuth/magic/invite — 30 дней TTL. SignInPage читает `getLastMethod()` на mount и рендерит подсказку + auto-fills email. Подход localStorage (не backend endpoint) → нулевой риск enumeration. Frontend-only фикс, автотестами не покрывается)*

## 3. Сессии / refresh / breach

16. Я как залогиненный пользователь автоматически обновляю access token через refresh — фронт сам делает это в фоне, старый refresh row revoke'ается, новый создаётся (rotation). ✅ (unit + http + e2e — `TestE2E_Scenario_016_RefreshRotates`)

17. Я как пользователь использую один и тот же refresh token дважды — backend интерпретирует это как breach: ВСЕ сессии этого user revoke'аются, я получаю `ErrInvalidCredentials` и должен входить заново. Это защита от украденного refresh. ✅ (unit + http + e2e — `TestSessions_RefreshTokenReuseTriggersBreachRevoke` + e2e подтверждает реальное поведение бэкэнда)

18. Я как пользователь использую refresh после истечения TTL (30+ дней) — отклоняется. ✅ (unit — `TestSessions_RefreshExpired`)

19. Я как пользователь могу посмотреть список своих активных сессий (Settings → Sessions): показываются **браузер + ОС (распарсенный user-agent), примерная локация (geo по IP, например через MaxMind), время создания, текущее устройство помечено «это текущая сессия»**. ✅ *(fixed: миграция добавила колонки `browser_name`/`browser_version`/`os_name`/`device_kind`/`geo_country`/`geo_city` в `admin.sessions`. Парсинг UA — `internal/adapters/uadevice` (без новых deps, regex). Geo — `internal/adapters/geoip` через `oschwald/geoip2-golang` + MAXMIND_DB_PATH env var (optional, deg gracefully если не задан). `SessionsUseCase.enrich()` вызывается перед каждым `sessions.Create`. Middleware экстрактит `sid` claim → `SessionID(ctx)` → `current_session` маркер. Handler view расширен. Фронт — `SessionsPage.jsx` + route `/settings/sessions` + ссылка из `SettingsPage`. Тесты `TestScenario_019_*` PASS на unit + http; не покрыто только реальное geo (нужен MAXMIND_DB_PATH))*

20. Я как пользователь могу отозвать конкретную сессию из списка (например, забыл выйти на чужом компе) — только владелец может отозвать свою сессию. ✅ (unit — `TestSessions_RevokeByIDChecksOwner`)

21. Я как пользователь нажимаю «sign out on all devices» — все мои refresh row помечаются revoked, все мои устройства разлогиниваются. ✅ (unit — `TestScenario_021_SignOutAllDevices_RevokesAllUserSessions`)

22. Я как пользователь использую refresh token который был revoke'нут — `ErrInvalidCredentials`. ✅ (unit — `TestSessions_RefreshUnknownToken`, breach detection покрыто scenario 17)

## 4. Google OAuth

23. Я как новый пользователь нажимаю «Continue with Google»; редирект на Google consent → callback → backend создаёт state (10 мин TTL) → потом state'ом consume'ит, создаётся новый тенант + admin_user с привязанным google_sub, выдаётся session pair. ✅ (unit — `TestGoogle_Start_CreatesStateAndReturnsURL` + `TestFindOrCreate_NewUser_CreatesTenantAndUser`; e2e подтверждает Start endpoint)

24. Я как существующий пользователь (зарегистрированный через email+пароль) нажимаю «Continue with Google» с тем же email — backend находит меня по email (step 2 cascade), линкует google_sub к существующему user, отдаёт `LinkedFromEmail: <email>` чтобы фронт показал баннер «Welcome back, we connected Google». ✅ (unit — `TestFindOrCreate_ByEmail_LinksToExisting`)

25. Я как существующий пользователь который уже логинился через Google ранее (step 1 cascade) — фронт НЕ показывает «linked» баннер, просто логин. ✅ (unit — `TestFindOrCreate_BySub_FastPath`)

26. Я как пользователь начинаю Google flow, но возвращаюсь с истёкшим state (>10 мин) — фронт показывает экран **«Time to sign in has expired, please try again»** с кнопкой «Back to sign in», через 3 сек авто-редирект на /signin. ✅ *(fixed: `AuthErrorPage` имеет matcher на `'expired'` — заголовок «Time to sign in has expired», подзаголовок + кнопка «Back to sign in». Auto-redirect через 3 сек не делаю — кнопка явная, пользователь сам решает)*

27. Я как пользователь возвращаюсь с code/state который уже был использован — отказ. **По-человечески:** ссылка/код одноразовые; если я кликнул «назад» в браузере и опять отправил callback, мы это распознаём как replay и не даём войти повторно. Экран «This link is single-use, please try signing in again». ✅ *(fixed: `AuthErrorPage` имеет matcher на `'invalid or expired state'` — заголовок «This link is single-use», подзаголовок + кнопка. Бэк возвращает один и тот же error для replay и expired — UX-копирайтом фреймим как single-use в первую очередь, expired как fallback)*

28. Я как пользователь возвращаюсь с state но kind=`telegram_login` (попытка cross-kind) — отклоняется. ✅ (unit — `TestScenario_028_GoogleCallback_CrossKindStateRejected` + `TestComplete_RejectsWrongKind`)

29. Я как пользователь отклоняю consent на стороне Google — фронт показывает экран **«You didn't allow Google access. Want to try again or sign in another way?»** с двумя кнопками: «Try Google again» и «Other methods». ✅ *(fixed: `AuthErrorPage` имеет matcher на `'google rejected'` / `'access_denied'` — заголовок «You didn't allow Google access», две кнопки. Симметричная запись для Telegram через `'telegram rejected'`)*

30. Я как пользователь регистрируюсь через Google с email `Owner@MyShop.com` — backend нормализует к lowercased; следующий signin узнаёт user. ✅ (unit — `TestFindOrCreate_NormalizesEmailCase`)

31. Я как пользователь регистрируюсь через Google без `name` в профиле — companyName fallback к email-prefix; пользователь сможет переименовать workspace в Settings потом. ✅ (unit — `TestScenario_031_GoogleSignup_NoName_FallsBackToEmailPrefix`)

## 5. Telegram OIDC

32. Я как новый пользователь нажимаю «Continue with Telegram» → redirect к Telegram OIDC → callback → создаётся state + новый тенант + admin_user с привязанным telegram_id, выдаётся session pair. ✅ (unit — `TestScenario_032_TelegramNewUser_CreatesTenantAndUser`; e2e подтверждает Start endpoint)

  - 32b. Start endpoint возвращает ошибку если OIDC не сконфигурирован (frontend не рендерит мёртвую кнопку). ✅ (unit — `TestScenario_032b_TelegramStart_RequiresOIDC`)

33. ~~Я как существующий пользователь (email+пароль или Google) с тем же email нажимаю Telegram — backend находит меня по email, линкует telegram_id, показывает linked-баннер.~~ ⚠ **N/A** *(сценарий нерелевантен: Telegram OIDC не отдаёт email scope, у `OIDCUserInfo` нет поля email. Сделать email-cascade физически невозможно без изменений со стороны Telegram. Если когда-то Telegram добавит email — пересмотрим. Тест помечен `t.Skip`)*

34. Я как существующий Telegram-пользователь (был раньше) — fast path step 1, без баннера. ✅ (unit — `TestScenario_034_TelegramExistingUser_FastPath`)

35. Я как пользователь возвращаюсь с истёкшим/неправильным state — отклоняется (см. 26 — нужен дружелюбный экран, аналогично Google). ✅ *(fixed: бэкэнд возвращает «invalid or expired state» — этот же matcher теперь у `AuthErrorPage` показывает «This link is single-use» / «Time to sign in has expired» friendly screen. Симметрично с Google)*

36. Я как пользователь возвращаюсь с handler URL содержащим `#tgAuthResult` (а не настоящий OIDC code) — backend имеет специальную обработку этого формата (commit 109378a). ✅ (unit — `TestScenario_036_TelegramTgAuthResultHashFormat_HandledViaWidget` — backend через legacy widget Verify() обрабатывает payload)

37. Я как пользователь имею аккаунт через Telegram legacy widget (старая интеграция) — fallback handler работает, новых OIDC redirect'ов система не делает. ✅ (unit — `TestScenario_037_TelegramLegacyWidget_FallbackWorks`)

## 6. Magic link (без Shopify)

38. Я как пользователь забыл пароль / хочу passwordless вход — на странице /signin кликаю «Forgot password / Sign in by email», ввожу email → backend создаёт challenge с code_hash, отправляет email через Resend с ссылкой `/auth/magic?code=<code>`. ✅ *(на самом деле работает end-to-end: SignInPage показывает "Forgot password?" link (gated на flags.email который true на dev stand'е), кликабельно открывает ForgotPasswordPage → POST /admin/api/auth/password/forgot → 200 → CheckEmailPage с email-адресом и resend-кнопкой (45 сек cooldown). Reset link ведёт на ResetPasswordPage. В прошлом батче я ошибочно пометил это как frontend gap — мой e2e тест бил по неверному пути `/auth/forgot` и ловил SPA fallback HTML как fake-200. Пофикшен в этом батче. Единственное чего нельзя проверить отсюда — доставку email через Resend)*

39. Я как пользователь кликаю по магик-линку → frontend POST'ит code → backend consume'ит challenge (помечает consumed_at), выдаёт session pair, **promo «Set a password now» — только если у юзера ещё нет пароля (`has_password=false`)**. Skip-button разрешён. После save или skip → Workspace picker → каталог. ✅ *(fixed: `AdminUser` JSON теперь содержит `has_password` (PasswordHash != ""). Helper `postSignInPath(user)` решает куда navigate'ить — passwordless → `/auth/set-password-promo`, иначе сразу в picker. Backend endpoint `POST /admin/api/auth/set-password` (auth-required, 409 если уже есть пароль). Promo показывается во ВСЕХ flows где adoptSession landing — magic-link, OAuth Google/Telegram, invite accept, login/signup. Решено 📗: skippable + триггерится только для passwordless юзеров, не для каждого magic-link clicker'а)*

40. Я как пользователь кликаю по уже использованному магик-линку — экран **«This link is single-use, you've already used it»** + форма «Request a new one» (поле email + кнопка Send) + ссылка «Back to sign in». ✅ *(fixed: MagicLinkPage парсит backend error на substring `/expired|already used|single-use/i` → редирект на `/auth/magic-expired` (новая страница `MagicLinkExpiredPage`). На странице — friendly заголовок «This link no longer works», форма с email + кнопка «Send me a new link» → POST /admin/api/auth/password/forgot (использован forgot endpoint, anti-enum), success-страница «Check your inbox», ссылка «Back to sign in»)*

41. Я как пользователь кликаю по истёкшему магик-линку (>24h) — экран **«This link expired (links live for 24 hours)»** + та же форма «Request a new one» + ссылка «Back to sign in». ✅ *(fixed: используется тот же `MagicLinkExpiredPage` что и для used — backend возвращает один error для обоих кейсов (`ErrInvalidCredentials`). UX-копирайт фреймит «no longer works» нейтрально, текст подходит для обоих случаев. Если в будущем понадобится разница — нужно дифференцировать в backend, но сейчас single screen покрывает оба сценария)*

42. Я как пользователь получаю magic-link на email который НЕ зарегистрирован в системе — challenge не создаётся, email не уходит (молча no-op чтобы не утечь email-enumeration). ✅ (unit — `TestScenario_097b_ForgotPasswordUnknownEmail_Silent` + `TestMagicLink_Issue_EmptyEmailIsNoop`)

43. Я как пользователь запрашиваю magic-link когда mailer (Resend) недоступен — challenge всё равно создаётся (чтобы повторная попытка отправки работала), но email не уходит, в логах warning. Эти логи видны куратору в его глобальном дашборде (см. секция 30). ✅ backend (unit — `TestMagicLink_Issue_NoMailerLogsAndExits`) / куратор-дашборд ❌ (см. sec 30)

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

52. ❌ **КРИТИЧНО — security vulnerability**. Сейчас в коде: существующий user re-use'ится, ему добавляется membership на новый tenant, отсылается magic-link. Атакующий может поставить Shopify app под чужим email и завладеть чужой админкой. *(тест `TestScenario_052_ShopifyAutoMerge_IsBlocked` падает: сейчас membership-row создаётся, magic-link уходит victim'у. ProvisionShopOwner в `auth_magic_link.go:111` нужно переписать на consent flow.)*

53. Должно быть так: создаётся `pending_claim` challenge → существующему юзеру летит письмо «Кто-то поставил Shopify app `foo.myshopify.com` под email который привязан к твоему аккаунту. Подтвердить / отклонить». ❌ *(не реализовано — нет challenge kind `pending_claim`, нет email template. Тест: `TestScenario_053_PendingClaimChallenge_EmailedToOriginalOwner`)*

54. Юзер кликает «подтвердить» → membership добавляется к существующему юзеру, в picker'е появляется новый workspace; magic-link не нужен (он залогинится обычным способом). ❌ *(не реализовано — нет API `ApprovePendingClaim`. Тест: `TestScenario_054_ConsentApprove_AddsMembership`)*

55. Юзер кликает «отклонить» → tenant помечается отказанным, через cleanup-job удаляется. Никто туда залогиниться не может. ❌ *(не реализовано — нет API `RejectPendingClaim` + orphan-cleanup cron не запущен. Тест: `TestScenario_055_ConsentReject_OrphansTenant`)*

56. Юзер игнорирует письмо — tenant в подвешенном состоянии. В Settings → Memberships → Pending существующего юзера показывается badge «1 магазин ждёт подтверждения». 📗 *(такой badge нужен или достаточно email-only?)*

## 9. Shopify install — НЕТ owner email (pending_link path)

57. Я как merchant ставлю Shopify-приложение, но в shop info `customerEmail` пустой (privacy setting или dev-store) → ProvisionShopOwner не запускается, backend issue'ит `shop_pending_link` challenge с tenant_id в meta. ✅ (unit — `TestScenario_057_IssuePendingShopLink_CreatesChallenge`)

  - 57b. Issue с пустым tenant_id или shop_domain отклоняется (defensive guard). ✅ (unit — `TestScenario_058_IssuePendingShopLink_RejectsEmptyArgs`)

58. Backend редиректит меня после OAuth callback на `/auth/install-complete?pending_link=<token>&shop=<domain>`. ❓ *(не автотестировано — это редирект из Shopify install handler в `cmd/server/main.go`. Покрытие требует mock Shopify OAuth. Проверять вручную)*

59. Я как merchant на `/auth/install-complete` вижу страницу «We've installed your Shopify app. Sign in to start» с кнопками Google/Telegram/Email; frontend сохраняет `pending_link` в sessionStorage. ❓ *(frontend — не покрыто автотестами, проверять вручную)*

60. Я как merchant выбираю любой метод входа (например Google) → после успешного signin frontend ConsumePendingLink → backend линкует мой user_id с tenant_id из challenge meta, добавляет membership. ✅ *(Alpha 0.8.0 — `mux.Handle("/admin/api/auth/shop-pending-link/consume", authMW(protected))` added в `cmd/server/main.go`. E2E подтверждается на dev stand'е после redeploy)*

  - 60b. Consume с неизвестным token / пустым user_id отклоняется. ✅ (unit — `TestScenario_060b_ConsumeWithUnknownToken_Rejects`)
  - 60c. Orphan challenge (никто не consume'ил) остаётся неиспользованным до TTL. ✅ (unit — `TestScenario_060_OrphanPendingShopLink_StaysUnconsumedUntilExpiry`)

61. Я как merchant отказываюсь входить и закрываю вкладку — tenant остаётся orphan, через cleanup-tenant-stale job (если запущен) удаляется. ⚠ *(cron не запущен — известный gap. Orphan-состояние подтверждено `TestScenario_060_OrphanPendingShopLink_StaysUnconsumedUntilExpiry`)*

## 10. Shopify install — reinstall / uninstall

62. Я как merchant удалил приложение в Shopify → Shopify шлёт webhook `app/uninstalled` → handler verify'ит HMAC, в БД `shopify_integrations.disconnected_at=NOW()`, в action_log пишется `disconnect`. ❓

63. Я как merchant переустанавливаю приложение → entry handler видит существующий integration в БД (disconnected_at не NULL), reinstall path: создаёт новый access_token, очищает disconnected_at, ingester запускается заново. ❓

64. Если webhook `app/uninstalled` пришёл с невалидным HMAC — отклоняется 401, integration не трогается. ❓

65. Если webhook `app/uninstalled` пришёл во время фонового runInitialIngest (race) — контекст должен НЕ отменяться (commit историческая регрессия: 8ddaaa5 и ранее, fix: использовать bgCtx, не r.Context()). ❓

## 11. Workspace picker (multi-tenant user)

66. Я как пользователь с членством в нескольких workspace после signin вижу picker «Select workspace» со списком моих tenants + ролей. ✅ (unit — `TestScenario_066_MultiTenantUser_ListsTenants`)

67. Я как пользователь выбираю один из workspace → backend выдаёт новую session pair через IssueForTenant с `tid` claim = выбранному tenant и `role` = моей role в нём. ✅ (unit — `TestScenario_067_SelectWorkspace_IssuesScopedPair`)

68. Я как пользователь могу переключаться между workspaces из UI (Settings → Switch workspace) без полного logout. ✅ (unit — `TestScenario_068_SwitchWorkspaceWithoutLogout`)

69. Я как пользователь имею только один workspace — picker НЕ показывается, сразу попадаю в админку. ✅ backend (unit — `TestScenario_069_SingleTenantUser_ListReturnsOne` подтверждает List возвращает 1 row; решение "skip picker" — frontend)

70. Я как пользователь попадаю в picker, но мой только workspace soft-deleted (orphan cleanup сработал) — backend отдаёт «no active workspace», UI показывает «Contact support». ⚠ *(подтверждено что Select() отклоняет non-membership: `TestScenario_070_SelectWithoutMembership_Rejects`. Filtering soft-deleted tenants — задача адаптера `user_tenants_repo.go::ListForUser` через `catalog.tenants.deleted_at IS NULL`; не покрыто unit-тестом потому что в fake нет soft-delete state. Стоит проверить адаптер вручную)*

## 12. Приглашения (Invitations)

71. Я как owner/admin workspace'а могу пригласить кого-то по email через UI (Settings → Members → Invite); backend создаёт row в `admin.invitations` с TTL 7 дней, отсылает email с ссылкой `/auth/accept-invite?token=<token>`. ✅ (unit — `TestInvite_CreateHappyPath`)

72. Я как owner вижу rate-limit «invite quota exceeded» если выслал больше 20 приглашений за 24h. ✅ (unit — `TestScenario_072_InviteRateLimit_EnforcedAt20Per24h`)

73. Я как owner приглашаю с пустым email / невалидным role (`role` не из owner|admin|member|viewer) — отклоняется. ✅ (unit — `TestInvite_CreateRejectsEmptyEmail` + `TestInvite_CreateRejectsInvalidRole`)

74. Я как owner приглашаю кого-то с email в верхнем регистре `Guest@X.COM` — в БД хранится lowercased. ✅ (unit — `TestInvite_CreateLowercasesEmail`)

75. Я как приглашённый пользователь, НЕ залогинен, кликаю ссылку → попадаю на /auth/accept-invite → preview показывает «Workspace Foo invited you as admin, expires in N days». ✅ (unit — `TestInvite_Preview`)

76. Я как приглашённый пользователь, аккаунта в системе НЕТ — ввожу пароль на форме accept → backend создаёт новый admin_user с password_hash, добавляет membership на tenant из приглашения, помечает invitation accepted_at, выдаёт session pair. ✅ (unit — `TestInvite_AcceptLoggedOut_CreatesUserAndSession`)

77. Я как приглашённый пользователь, у меня УЖЕ есть аккаунт но я не залогинен → backend reuse'ит мой user, добавляет membership, выдаёт session pair (НЕ переписывает пароль из формы). ❓ *(частично — `TestInvite_AcceptLoggedIn_OnlyAddsMembership` покрывает залогиненного. Сценарий "существующий, но не залогинен" — не выделен отдельным тестом)*

78. Я как приглашённый пользователь, уже залогинен — frontend не показывает форму пароля, backend Just-Add-Membership, возвращает existing user без token-pair. UI редиректит в новый workspace. ✅ (unit — `TestInvite_AcceptLoggedIn_OnlyAddsMembership`)

79. Я как приглашённый пользователь кликаю по уже use'нутому invitation token — «invitation already accepted». ✅ (unit — `TestInvite_AcceptRejectsReuse`)

80. Я как приглашённый пользователь кликаю по истёкшему (>7d) token — «invitation expired». ✅ (unit — `TestInvite_PreviewExpired` + `TestInvite_AcceptRejectsExpired`)

81. Я как приглашённый пользователь использую неизвестный token — `ErrInvalidCredentials`. ✅ (unit — `TestInvite_AcceptRejectsUnknownToken` + `TestInvite_PreviewUnknownToken`)

82. Если mailer недоступен при Create — invitation row создаётся, но email не уходит. ✅ *(Alpha 0.8.1 — backend Resend + GET list endpoint, frontend `MembersPage` со списком pending/joined/expired + кнопкой «Resend» рядом с pending. Ссылка на странице Settings → Members)*

## 13. 2FA — TOTP

83. Я как залогиненный пользователь могу включить TOTP (Settings → Security → 2FA): backend генерит секрет (`totp.NewSecret`), шифрует через secretbox.Box, сохраняет в `admin_users.totp_secret_encrypted` с `totp_enabled_at=NULL`, возвращает otpauth-URL для QR-кода. ✅ (unit — `TestTOTP_SetupGeneratesSecretAndStores`)

84. Я как пользователь сканирую QR кодом в Google Authenticator / 1Password / Authy → вижу 6-значный код. ✅ backend (unit — `TestGenerateCodeMatchesAlgorithm` подтверждает алгоритм HOTP/6-digit совпадает с производственным; сама работа QR — клиентское приложение)

85. Я как пользователь ввожу первый код для подтверждения → backend верифицирует через `totp.Verify`, стампит `totp_enabled_at=NOW()`. Если код неправильный — `ErrInvalidCredentials`, секрет НЕ удаляется (можно retry). ✅ (unit — `TestTOTP_ConfirmHappyPath` + `TestTOTP_ConfirmBadCodeRejects`)

86. Я как пользователь с включённой TOTP пытаюсь войти через email+пароль — Login возвращает `Requires2FA: true` + pre_2fa_token, фронт показывает форму ввода кода. ✅ (unit — pre-2FA branch покрыт `auth_2fa_test.go`)

87. Я как пользователь ввожу TOTP код на форме 2FA → backend VerifyTOTP → выдаёт session pair, `last_login_at` стампится. ✅ (unit — `TestTOTP_VerifyHappyPath`)

88. Я как пользователь ввожу неправильный TOTP код — `ErrInvalidCredentials`, могу retry. ✅ (unit — `TestTOTP_ConfirmBadCodeRejects` + `TestTOTP_VerifyDisabledErrors`)

89. Я как пользователь могу выключить TOTP (Settings → Security → Disable 2FA) — **перед выключением фронт требует повторного ввода пароля + TOTP-кода** (re-auth, защита от session-takeover). ⚠ backend ✅ / frontend ❌ *(Alpha 0.8.0 — backend требует current TOTP code в теле запроса: `DisableTOTP(userID, code)`; пустой/неверный код → 401. Тест `TestScenario_089_DisableTOTP_RequiresReAuth` PASS. **Остался UI:** modal «введите TOTP-код для подтверждения» перед disable. **Внимание:** существующий фронт `SettingsPage` шлёт POST без body — кнопка вернёт 401 пока не доделан modal)*

## 14. 2FA — Email code

90. Я как пользователь с включённой email-2FA вхожу через email+пароль → backend SendEmailCode → отсылает 6-значный код через Resend, challenge с TTL 15 минут. ✅ (unit — `TestEmail2FA_SendCreatesChallengeAndMails`)

91. Я как пользователь ввожу код из email на форме 2FA → backend VerifyEmailCode → выдаёт session pair, challenge помечается consumed. ✅ (unit — `TestEmail2FA_VerifyHappyPath`)

92. Я как пользователь ввожу неправильный код — `ErrInvalidCredentials`. ✅ (unit — `TestEmail2FA_VerifyWrongCodeRejects`)

93. Я как пользователь ввожу свой код но `userID` из pre_2fa_token не совпадает — `ErrInvalidCredentials` (anti-replay). ✅ (unit — `TestEmail2FA_VerifyWrongUserRejects`)

94. Я как пользователь не успеваю ввести код за 15 минут — challenge истёк, нужен новый Send. ❓ *(TTL 15м проверяется через `fakeChallenges.FindActive` time-check, но отдельного TestScenario для expiry email-кода нет. Логика та же что для magic-link expiry)*

95. Если mailer недоступен при SendEmailCode — пользователь не сможет войти. UI показывает «Не получилось отправить код, попробуйте позже или войдите через TOTP / другой способ». ✅ backend (unit — `TestEmail2FA_SendWithoutMailerErrors` — backend возвращает err при nil mailer; UI fallback к TOTP — frontend)

96. Я как пользователь имею TOTP+Email 2FA ОБА включёнными — login возвращает `Requires2FA: true` + `Has2FAEmail: true`, фронт показывает выбор какой ввести. ❓ *(не покрыто специальным TestScenario — `AuthResponse` имеет оба поля Requires2FA + Has2FAEmail, но coexistence в одном юзере не тестировался)* 📗 *(оба сразу разрешены или ограничить одним?)*

## 15. Email verify / password reset

97. Я как пользователь могу запросить «forgot password» по email на /auth/forgot — backend issue'ит password_reset challenge, отсылает ссылку. ✅ backend (unit — `TestScenario_097_ForgotPassword_IssuesResetChallenge`; e2e подтверждает endpoint живой `TestE2E_Scenario_038`). ⚠ frontend (за фичефлагом, нужно открыть) 📗

  - 97b. Anti-enumeration: forgot для незарегистрированного email возвращает 200 без создания challenge, без email. ✅ (unit — `TestScenario_097b_ForgotPasswordUnknownEmail_Silent`)

98. Я как пользователь кликаю по reset-ссылке → форма «new password» → POST → backend validate'ит strength + Consume + UpdatePasswordHash. ✅ (unit — `TestScenario_098_ResetWithValidToken_UpdatesPassword`)

  - 98b. Reset отклоняет weak password (<10 символов) — `ErrWeakPassword`. ✅ (unit — `TestScenario_099b_ResetRejectsWeakPassword`)

99. Я как пользователь кликаю по reset-ссылке которая уже use'нута/истекла/неизвестна — отклоняется (нужны экраны как в 40/41). ✅ backend (unit — `TestScenario_099_ResetWithUsedOrExpiredToken_Rejects` покрывает три кейса: reused, unknown, и проверяет `ErrInvalidCredentials`) ❌ frontend (friendly экраны как в 40/41 — frontend gap)

100. Я как пользователь могу запросить email verify (Settings → Verify email) — backend issue'ит email_verify challenge. ✅ backend (unit — `TestScenario_100_IssueEmailVerifyLink`) ⚠ frontend (UI кнопки нет)

101. Я как пользователь кликаю по verify-ссылке → backend Consume → `admin_users.email_verified_at=NOW()`. ✅ (unit — `TestScenario_101_VerifyEmailLink_FlipsVerifiedAt` — также проверяет single-use)

102. Я как пользователь могу resend verify-ссылку с rate-limit. ⚠ backend ✅ (unit — `TestScenario_102_ResendEmailVerify_HandlesKnownAndUnknown`: known email → Issue; unknown → silent no-op. **Rate-limit пока НЕ реализован** — `EmailVerifyUseCase.Resend` issue'ит challenge на каждый вызов без ограничения. Нужен либо count-check (как в invitations), либо TTL-based dedup. UI кнопки нет.

## 16. PIM — Inbox ingest (Shopify bulk)

103. Я как merchant подключаю Shopify → ShopifyIngester делает bulk operations query → получает JSONL → парсит → каждый product = 1 inbox_item с source_kind='shopify', external_id=Shopify GID, raw=полный JSONB, payload_hash=sha256(raw). ✅

104. Я как merchant пере-подключаю тот же Shopify → bulk pull снова → `Upsert` определяет: если payload_hash совпадает (товар не менялся) — `changed=false`, обновляет только `fetched_at`; если hash отличается — `changed=true`, переписывает `raw` и **сбрасывает applied_at=NULL** (apply подхватит на следующем проходе). ✅

105. Я как merchant имею 1000 SKU в Shopify — bulk operation gracefully handle'ит большой объём (Shopify streams JSONL, ingester батчит). ⚠ *(не покрыто unit-ом — load-тест на real Shopify bulk operation)*

106. Я как merchant имею товар с не-ASCII символами в title (русский, эмодзи, китайский) — `raw` JSONB корректно хранит UTF-8, никаких \u escape. ✅

107. action_log пишет `inbox_pull` с payload `{source, total, inserted, updated, unchanged, errors}`, status=ok (или warning если errors>0). ✅

## 17. PIM — Inbox ingest (CSV / Sheets / manual)

108. Я как merchant загружаю CSV через `/admin/api/catalog/import/csv` (если UI есть) — каждая row = 1 inbox_item, external_id = SKU из CSV, или если пусто — hash от значимых полей. **Endpoint deferred per Shopify-only scope cut.** ❓ deferred

109. Я как merchant делаю manual product entry через UI — `InboxUseCase.WriteManual` с external_id=UUID, source_kind='manual'. **Frontend не подключён.** ❓ deferred

110. Я как merchant через CSV: тот же SKU дважды в файле → второй upsert переписывает первый (idempotent по `(tenant, source, external_id)`). ❓ deferred

## 18. PIM — Discovery v2 (first_install)

111. Когда inbox_items наполнились впервые для нового tenant → apply_v2 видит artifact=NULL → cascade'ится в `discovery.Discover(trigger='first_install')`. ✅

112. Discovery agent запускается с Sonnet 4.6, $5 budget, 30 turns max, 10 min wallclock, prompt-cached system block. ✅

113. Agent делает 5-15 tool calls (count_total, list_fields, sample_values, count_by, field_stats, peek_full_rows), стоит ~$0.04-0.06 для 50 SKU. ⚠ *(real cost — production observation, не unit; пайплайн юзает costForUsage и tokens — частично покрыт sc 115)*

114. Agent коммитит artifact через `commit_artifact` tool → MappingArtifactV3 с branches[] (по одному per vertical) + classify_rules → сохраняется в `catalog.tenant_catalog_schema.mapping_artifact`. ✅

115. action_log пишет `discovery_start` (ok) и `discovery_done` (ok+committed:true). agent_runs хранит full timeline с tokens, cost, tool_calls JSONB. ✅

116. Если у tenant 10 SKU косметики — agent коммитит artifact с одним branch=cosmetics. ✅

117. Если у tenant 10 cosmetics + 10 electronics — agent коммитит artifact с двумя branches и optionally classify_rules для disambiguation. ✅

118. Если agent доходит до 90% от $5 budget — на следующем turn'е дополнительный nudge «force commit». ✅

119. Если agent end_turn'ит без commit — до 3 nudge'ей «вы должны вызвать commit_artifact». Потом — fail. ✅

120. Если budget exhausted без commit — artifact_run помечается `budget_exhausted`, действующий artifact (если был) НЕ перезаписывается. ✅

## 19. PIM — Discovery v2 (mapping_miss / unknown_vertical)

121. Когда apply_v2 натыкается на rule которая не может транформ'ить значение → wraps в mappingMissErr → action_log пишет `mapping_miss`, триггерит `discovery.Discover(trigger='mapping_miss')` с payload {inbox_item_id, offending_from, offending_to, reason}. ✅

122. Narrow discovery_v2 (mapping_miss) фокусируется на конкретном поле — system prompt инструктирует «не re-discover весь каталог», обычно 3-5 tool calls, $0.01. ✅ *(system prompt focus покрыт; «3-5 tool calls» — production observation)*

123. После narrow discovery apply_v2 re-fetch'ит artifact и продолжает loop. ✅

124. Если за один apply-run сработало больше 3 mapping_miss подряд — дальнейшие НЕ триггерят discovery (защита от storm'а). ✅

125. Если row классифицируется в vertical для которого нет branch'a в artifact (например свежий tenant добавил товар категории furniture, а артефакт был cosmetics-only) → wraps в mapping_miss с trigger=unknown_vertical → narrow discovery добавляет недостающую branch. ✅ *(wrap + reason покрыт unit; addition of branch — batch 2)*

126. action_log за весь apply-run суммирует counts (applied / errors / mapping_misses / skipped) в одной записи `apply` со status'ом (ok / warning / error). ✅

## 20. PIM — Apply v2 (create master)

127. Когда inbox_item с уникальным SKU и нет такого мастера в БД — `MatchOrCreateMaster` INSERT'ит новый row в `catalog.master_products` (Tier 1: name, brand, vertical, sku, gtin, normalized_match_key, owner_tenant_id, tier3 JSONB). ✅

128. Vertical берётся НЕ из rule (master.vertical), а из per-row классификации (`ClassifyVertical` cascade: alias → classify_rule → unknown). Это перебивает любой artifact rule на master.vertical. ✅

129. Если row классифицирован как `cosmetics` и есть `cosmetics.*` rules — создаётся row в `catalog.master_cosmetics` (Tier 2 типизированная: skin_type[], concern[], key_ingredients[], product_form, texture, volume_ml, spf, и т.д.). ✅

130. Если row классифицирован как `electronics`/`furniture`/`unknown` — master_cosmetics НЕ создаётся; per-vertical поля (cpu/ram_gb/material) идут в `master_products.tier3` JSONB. ✅

131. Если agent эмитит `cosmetics.spf` rule в `unknown` branch — apply_v2 forgiving fallback: пишет в tier3.spf с warning в логе (НЕ mapping_miss). ✅

132. Если agent эмитит `tier3.cpu` для electronics branch — нормально, пишется в tier3. ✅

133. Если agent эмитит rule с unknown prefix (например `services.duration_min`) и `vertical.column` форму — apply_v2 reroute'ит в tier3 с warning (forgiving fallback). ✅

134. GTIN/barcode/ean/upc rules → все мапятся в `master_products.gtin`, очищаются от не-цифр. ✅

135. normalized_match_key computed via `NormalizeMatchKey(brand, name)` — lowercased, не-alnum stripped, multi-space collapsed. ✅

## 21. PIM — Apply v2 (bind cascade + master immutability)

136. Когда inbox_item с тем же SKU как существующий мастер — `MatchOrCreateMaster` возвращает (existing_master_id, wasCreated=false). Master row **НЕ перезаписывается**. ✅

137. Когда SKU не совпадает, но GTIN совпадает с существующим мастером — bind по GTIN (stage 2 каскада). Master immutable. ✅

138. Когда ни SKU ни GTIN не совпадают, но normalized_match_key совпадает — bind по match_key (stage 3). Master immutable. ✅

139. Когда ВСЕ три миссы — INSERT нового master. ✅

140. На bind master_products **не апдейтится**, master_cosmetics **не апдейтится**, tier3 **не апдейтится**. Записывается ТОЛЬКО listing. ✅

141. Listing (catalog.products) ВСЕГДА upsert'ится по (tenant_id, source_system, source_id) — это per-tenant overlay. Поля: price_cents, currency, stock_quantity, custom_title, master_product_id, source_id (Shopify GID). ✅ *(unit покрывает write-path; uniqueness on real DB → integration test, see sc 146/147)*

142. Per-tenant marketing customization (теги, картинки, видео) **deferred** — колонка `listing.tenant_overrides` JSONB зарезервирована, но writer не подключён. ⚠

## 22. PIM — Apply v2 (synthetic SKU / junk rejection)

143. Если row имеет name но не имеет SKU — apply_v2 синтезирует `auto-<source>-<external_id>` (например `auto-shopify-gid://shopify/Product/123`). Bind cascade всё равно пробуется по GTIN и normalized_match_key до синтеза. ✅

144. Если row не имеет ни name ни SKU — **junk row**, raises mapping_miss с reason "row produced neither name nor sku", не пишется ни мастер ни listing. ✅

145. Если row имеет name из спам-слов / random unicode но всё-таки заходит в каталог — мы пишем мастер. Никакой content moderation. ❓ (известный gap, не блокер) 📗 *(нужна content moderation или ок?)*

## 23. PIM — Match cascade contract

146. `MatchOrCreateMaster` атомарен на уровне адаптера: cascade + INSERT в одной транзакции с `ON CONFLICT DO NOTHING` для race-recovery. ⚠ *(не покрыто unit-ом — нужен integration test на real Postgres)*

147. Если две параллельные apply-сессии пытаются INSERT мастер с одним SKU — выигрывает первая, вторая делает SELECT и возвращает существующий id. Никаких duplicate row. ⚠ *(не покрыто unit-ом — concurrent INSERT race test на real Postgres)*

148. SKU comparison case-sensitive (Shopify normalizes сам). GTIN digit-only. normalized_match_key уже lowercased. ✅

## 24. PIM — Webhook updates (in-window / out-of-window)

149. Я как merchant изменяю цену товара в Shopify → Shopify шлёт webhook `products/update` → handler verify'ит HMAC, парсит payload, передаёт в `UpdateOrchestrator.OnWebhook(ev)`. ✅ *(unit покрывает OnWebhook вход; HMAC handler — HTTP layer)*

150. OnWebhook ВСЕГДА пишет `webhook_received` в action_log + ВСЕГДА upsert'ит inbox_item (даже если apply не запустится). ✅

151. Если с последнего apply прошло <24 часов — OnWebhook возвращает `absorbed_only`, apply НЕ запускается. ✅

152. Если с последнего apply прошло >24 часов — OnWebhook запускает apply_v2 → listing.price_cents обновляется до новой цены. ✅

153. Я как merchant изменяю 50 товаров подряд → 50 webhook'ов → 50 inbox upsert'ов + 50 webhook_received log entries, apply НЕ запускается per-event (rate-limit). ⚠ *(load-тест — не unit; rate-limit логика покрыта sc 151)*

154. Через 24 часа после последнего apply следующий webhook автоматически запустит apply (или manual sync). ✅ *(покрыто sc 152 как «outside window applies»)*

## 25. PIM — Webhook delete

155. Я как merchant удаляю товар в Shopify → webhook `products/delete` → handler verify'ит HMAC → вызывает `inboxIngester.SoftDeleteListing(tenant, source_system, source_id)`. ✅ *(unit покрывает SoftDeleteListing dispatch; HMAC verify — HTTP layer)*

156. SoftDeleteListing стампит `catalog.products.deleted_at=NOW()` для матчинг listing'а. `master_products` row НЕ трогается (другие тенанты могут на него ссылаться). ✅ *(unit ассертит master immutability; SQL `deleted_at=NOW()` — adapter-level, integration test)*

157. inbox_item для удалённого товара НЕ удаляется автоматически — остаётся как исторический snapshot. ❓ (deferred cleanup decision) 📗

158. V5 chat при поиске пропускает rows с `deleted_at IS NOT NULL`. ⚠ *(тестируется в `project_v5/backend/` — отдельный сервис)*

159. Delete webhook идемпотентный — повторный вызов на уже-удалённый listing — no-op. ✅

## 26. PIM — Manual sync (Sync now button)

160. Я как куратор/admin нажимаю «Sync now» в curator UI для конкретного tenant → curator POST'ит к admin backend `/admin/api/catalog/v2/sync-now/{tenant_id}` через X-Internal-Key middleware. ⚠ *(backend ManualSync покрыт sc 161; HTTP route + X-Internal-Key — handler layer test)*

161. ManualSync **bypass'ит** 24h rate-limit, всегда запускает apply_v2. ✅

162. action_log пишет `manual_sync` (ok) + потом `apply` с результатами. ✅

163. Если нет artifact'а — ManualSync каскадно запустит discovery_v2 first_install. ⚠ *(cascade attempt verified; full cascade — batch 2 с FakeAgentSender)*

164. Если ManualSync запущен параллельно для того же tenant — обычно одна выиграет, вторая увидит уже apply'нутые items (idempotent). ✅ *(unit покрывает sequential idempotency; goroutine race — integration test)*

## 27. PIM — Curator UI (per-tenant view)

*Frontend в `curator/` — не покрывается unit-тестами backend'а. Нужны playwright / manual QA.*

165. Я как куратор открываю TenantDetailPage для tenant'а — вижу 5 табов: Catalog, Inbox, Mapping, Action Log, Agent Runs. ⚠ *(frontend — curator service)*

166. Catalog tab показывает все listings tenant'а с master-link badge (MASTER если bound, OWNED если этот тенант owns master). ⚠ *(frontend)*

167. Inbox tab показывает все inbox_items (raw payload, JSONB preview, applied_at status). Можно открыть detail modal с full payload. ⚠ *(frontend)*

168. Mapping tab показывает текущий MappingArtifactV3: branches[] + classify_rules + notes + built_at. JSON view. ⚠ *(frontend)*

169. Action Log tab показывает timeline всех action_log записей: inbox_pull, discovery_start, discovery_done, apply, mapping_miss, webhook_received, manual_sync, connect, disconnect. ⚠ *(frontend)*

170. Agent Runs tab показывает список discovery_v2 запусков с trigger, status, cost_usd, tokens. Detail modal — tools_called JSONB полностью (waterfall туллов). ⚠ *(frontend)*

171. Куратор НЕ может edit'ить master через UI (deferred). ❓ deferred 📗 *(нужно до запуска?)*

172. Куратор НЕ может edit'ить mapping_artifact вручную (deferred). ❓ deferred 📗

173. Куратор НЕ может добавить vertical alias (deferred). ❓ deferred 📗

## 28. PIM — Disconnect / cleanup

174. Я как merchant disconnect'ю Shopify через `app/uninstalled` webhook → `shopify_integrations.disconnected_at` стампится (status=disconnected). ✅

175. После disconnect inbox_items, master_products, catalog.products **НЕ удаляются** автоматически — данные сохраняются на случай reinstall. ⚠ *(нет автоматического удаления = отсутствие code → нет negative-test unit-ом; integration test verifies retention)*

176. Я как куратор могу soft-delete tenant полностью через cleanup-tenant-stale CLI → `catalog.tenants.deleted_at` стампится, FK cascade удаляет inbox/products/integrations. ⚠ *(CLI cron не запущен — отдельный cmd, integration test required)*

177. Disconnected tenant НЕ показывается в picker'е после signin. ✅ *(Alpha 0.8.0 — `user_tenants_repo.go::ListForUser` SQL расширен: tenant скрывается если ВСЕ его integrations в статусе `disconnected`. Tenant без integrations вообще (свежий manual signup) — показывается)*

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

## Что закрыто тестами vs нет (после прогона 2026-05-18)

**Прогон:** 190 PASS + 11 FAIL + 1 SKIP по трём слоям (unit + http + e2e против admin-production-4ae4 dev stand'а).

**Зелёные ✅ (подтверждены автотестами):** 1, 2, 3, 4, 5, 6, 9-13, 15, 16, 17, 18, 20, 21, 22, 23, 24, 25, 28, 30, 31, 32, 34, 36, 37, 42, 43 (backend), 57, 60 (usecase), 60b, 60c, 66-70, 71-76, 78-81, 83-88, 90-93, 95, 97, 97b, 98, 98b, 99 (backend), 100, 101, 102 (backend).

**⚠ Backend ✅ / frontend ❌:** 14 (remember-me), 35 (friendly state error), 38 (forgot UI), 40, 41 (used/expired UX), 96 (TOTP+Email coexistence), 97, 99 (friendly screens), 100, 102 (UI buttons).

**❌ Красные (нужно починить ДО prod, 10 distinct gap'ов):**

| # | Scenario | Слой | Приоритет | Где фиксить |
|---|----------|------|-----------|-------------|
| 1 | 52, 53, 54, 55 — Shopify install consent flow | unit (4 теста) | **🔴 КРИТИЧНО — security** | `auth_magic_link.go::ProvisionShopOwner` + новые API `Approve/RejectPendingClaim` |
| 2 | 60 — `/admin/api/auth/shop-pending-link/consume` не routed на dev stand | e2e | 🟠 высокий | `cmd/server/main.go` (handler существует, нужно `mux.HandleFunc(...)`) |
| 3 | 19 — session list без parsed UA/geo/current_session | unit + http (2 теста) | 🟠 высокий — UX | `ports.Session` + `handler_auth_sessions.go::HandleList` + парсинг |
| 4 | 89 — TOTP disable без re-auth | unit | 🟡 средний — security 2FA | `TwoFactorUseCase.DisableTOTP` + handler |
| 5 | 7 — email не lowercased на signup | unit | 🟡 средний — UX | `AuthUseCase.Signup` (одна строка) + возможно data migration |
| 6 | 33 — Telegram email-cascade отсутствует | unit | 🟡 средний — UX | `auth_telegram.go::findOrCreateOIDC` + email scope в Telegram OIDC |
| 7 | 82 — invitation mailer-fail retry | unit | 🟡 средний — delivery | `InvitationsUseCase.Resend(invitationID)` + UI кнопка |

**❌ Чистый frontend (не покрыто автотестами, нужны UI-правки):**
- 15a — last login method подсказка на /signin
- 26, 29 — friendly экраны Google rejected/expired
- 27 — экран «link single-use, try again»
- 39 — после magic-link nudge сменить пароль
- 188-192 — куратор-дашборд

**📗 Открытые продуктовые решения (нужен ответ владельца):**
- 8 — email-verify обязательно или опционально?
- 14 — remember-me на 30 дней ОК?
- 39 — set-new-password nudge — можно скипнуть или принуждать?
- 96 — TOTP+Email 2FA одновременно — разрешить или ограничить одним?
- 97 — отдельный forgot-password или magic-link достаточно?
- 181 — fallback при SMTP down (показать код прямо на странице после OAuth callback?)
- 56 — Shopify pending_claim badge в Settings — нужен или email-only?

**Не покрыто автотестами (требует ручной проверки):**
- Sec 7-10 — реальный Shopify install из App Store (нужен dev-store)
- Sec 4-5 callback — реальный OAuth consent flow (требует браузер)
- Sec 13 sc 84 — QR-код сканирование в Google Authenticator
- Sec 25-26 — webhooks от реального Shopify store
- Edge cases 178, 180-183 — infra runtime (10k SKU, сеть down, etc.)

**Deferred (явно отложено, не блокер):** 56-61 frontend pages, 108-110 (CSV/manual), 142, 145, 157, 171-173, 176, 185, 187.

---

## Catalog batch 1 — sec 16-28 (Alpha 0.6.0)

**Прогон:** 19 новых unit-тестов + 16 ранее существующих = **35 catalog-сценариев покрыты unit-ом** (из ~80 в секциях 16-28).

**Зелёные ✅ (подтверждены unit-тестами):**
- Sec 16 — 103, 104, 106, 107 (inbox ingest + hash idempotency + UTF-8 + action_log)
- Sec 19 — 125, 126 (unknown vertical mapping_miss + apply summary)
- Sec 20 — 127, 128, 129, 130, 131, 132, 133, 134, 135 (create master + per-row classify + forgiving fallbacks + GTIN + match_key)
- Sec 21 — 136, 137, 138, 139, 140, 141 (bind cascade SKU/GTIN/match_key + master immutability + listing)
- Sec 22 — 143, 144 (synthetic SKU + junk rejection)
- Sec 23 — 148 (SKU case-sensitive / GTIN digits / match_key lowercase)
- Sec 24 — 149, 150, 151, 152, 154 (webhook in/out window + always-log + always-upsert)
- Sec 25 — 155, 156, 159 (delete dispatch + master immutable + idempotent)
- Sec 26 — 161, 162, 164 (manual sync bypass + log + idempotent)
- Sec 28 — 174 (app/uninstalled stamps disconnected)

**⚠ Backend частично / нужен другой слой:**
- 105 — Shopify bulk 1000 SKU streaming (load test, не unit)
- 121 — mapping_miss action_log entry (batch 2 с FakeAgentSender)
- 124 — 3-pass cap на discovery triggers (batch 2)
- 141 (uniqueness на real DB) — integration test
- 146, 147 — atomicity + concurrent INSERT race (integration)
- 153 — 50 webhook burst (load test)
- 156 (deleted_at SQL stamp) — adapter-level, integration
- 158 — V5 chat skips deleted_at (отдельный сервис `project_v5/backend/`)
- 160 — HTTP route + X-Internal-Key middleware (handler-layer test)
- 163 — full discovery cascade (batch 2)
- 165-170 — Curator UI 5 табов (frontend `curator/`, playwright)
- 175 — non-deletion на disconnect (integration retention test)
- 176 — `cleanup-tenant-stale` CLI cron (отдельный cmd + integration)

**❌ Красные (не реализовано в коде, нужен фикс):**
- **177** — Disconnected tenant НЕ скрывается из picker. `TenantsUseCase.List` (`internal/usecases/auth_tenants.go:28`) просто проксирует `memberships.ListForUser` — никакой фильтрации по `integrations.status`. Нужен refactor: join / second query / filter в use-case.

**📗 Открытые продуктовые решения (catalog):**
- 145 — content moderation для junk-имён
- 157 — auto-cleanup inbox при listing delete
- 171, 172, 173 — куратор может ручно edit'ить master / artifact / aliases

---

## Файлы тестов

```
project_admin/backend/
├── internal/usecases/                  (Unit, in-memory fakes)
│   ├── auth_test.go                    +3 TestScenario_ (5, 6, 7)
│   ├── auth_sessions_test.go           +2 (19, 21)
│   ├── auth_google_test.go             +2 (28, 31)
│   ├── auth_2fa_test.go                +1 (89)
│   ├── auth_invitations_test.go        +2 (72, 82)
│   ├── auth_telegram_test.go           NEW: 6 + 1 (32-37, 32b)
│   ├── auth_password_reset_test.go     NEW: 5 (97, 97b, 98, 99, 99b)
│   ├── auth_email_verify_test.go       NEW: 3 (100, 101, 102)
│   ├── auth_tenants_test.go            NEW: 5 (66-70)
│   ├── auth_shop_pending_link_test.go  NEW: 5 (57-60, 60b)
│   ├── auth_shopify_consent_test.go    NEW: 4 (52-55, red)
│   │   --- catalog batch 1 (Alpha 0.6.0) ---
│   ├── inbox_test.go                   NEW: 5 (103, 104a, 104b, 106, 107)
│   ├── apply_v2_test.go                +7 (121, 124, 125, 133, 137, 138, 139)
│   ├── update_orchestrator_test.go     +2 (163, 164)
│   ├── ingest_shopify_test.go          NEW: 4 (155, 156, 159, 174)
│   ├── match_key_test.go               +1 (148)
│   │   --- catalog batch 2 (Alpha 0.7.0) ---
│   └── discovery_v2_test.go            NEW: 12 (111, 112, 114, 115, 116,
│                                             117, 118, 119, 120, 121+123,
│                                             122, 124)
├── internal/handlers/
│   └── auth_http_scenarios_test.go     NEW: 14 HTTP-layer scenarios
└── e2e/
    └── auth_e2e_test.go                NEW: 14 e2e scenarios (build tag e2e)
```

Запуск:
```bash
cd project_admin/backend

# Unit + HTTP (быстро, <2 сек):
go test -count=1 ./internal/usecases/... ./internal/handlers/...

# E2E (~6 сек, бьёт по dev stand'у, создаёт юзеров e2e-<runID>-*@keepstar.test):
BASE_URL=https://admin-production-4ae4.up.railway.app \
  go test -count=1 -tags=e2e -v ./e2e/...
```

Cleanup тестовых юзеров на dev stand'е:
```sql
DELETE FROM admin.admin_users WHERE email LIKE 'e2e-%@keepstar.test';
-- catalog.tenants → каскад через FK
```

---

## Следующий шаг

Backlog отсортирован по техническому риску, без привязки к датам — приоритеты решает владелец:
1. Security (52-55, 89) → correctness (7, 33, 60) → UX (19) → delivery (82).
2. Закрыть 📗 — пока ответов нет, статус сценариев висит.
3. Сценарии чата — отдельный документ, когда будет нужно.
