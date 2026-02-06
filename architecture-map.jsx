import { useState } from "react";

const COLORS = {
  user: { bg: "#F0F9FF", border: "#0EA5E9", text: "#0369A1" },
  frontend: { bg: "#F0FDF4", border: "#22C55E", text: "#15803D" },
  handler: { bg: "#FFFBEB", border: "#F59E0B", text: "#B45309" },
  agent1: { bg: "#EFF6FF", border: "#3B82F6", text: "#1D4ED8" },
  agent2: { bg: "#F5F3FF", border: "#8B5CF6", text: "#6D28D9" },
  state: { bg: "#FFF1F2", border: "#F43F5E", text: "#BE123C" },
  db: { bg: "#ECFDF5", border: "#10B981", text: "#047857" },
  llm: { bg: "#FDF4FF", border: "#D946EF", text: "#A21CAF" },
  domain: { bg: "#F8FAFC", border: "#94A3B8", text: "#475569" },
  arrow: "#94A3B8",
};

const details = {
  user: {
    title: "Пользователь",
    what: "Человек, который пишет в чат",
    does: "Отправляет текстовые запросы: поиск товаров, смена стиля отображения, навигация по карточкам",
    examples: [
      '"покажи кроссовки Nike" → поиск',
      '"сделай заголовки крупнее" → смена стиля',
      'Клик на карточку → раскрытие деталей',
      'Кнопка "Назад" → возврат к списку',
    ],
  },
  chatpanel: {
    title: "ChatPanel (React)",
    what: "Окно чата в браузере",
    does: "Принимает текст пользователя, отправляет на бэкенд, показывает историю сообщений",
    data_in: "Текст от пользователя + sessionId",
    data_out: "POST /api/v1/pipeline { sessionId, query }",
    files: ["src/features/chat/ChatPanel.jsx", "src/shared/api/apiClient.js"],
  },
  formation_renderer: {
    title: "FormationRenderer (React)",
    what: "Рендерит виджеты в сетку/список/карусель",
    does: "Получает formation от бэкенда и раскладывает виджеты по лейауту. Каждый виджет рендерится через WidgetRenderer → AtomRenderer",
    data_in: 'FormationWithData { mode: "grid", widgets: [...] }',
    chain: "Formation → Widget → Atoms (image, text, price, rating...)",
    files: [
      "src/entities/formation/FormationRenderer.jsx",
      "src/entities/widget/WidgetRenderer.jsx",
      "src/entities/atom/AtomRenderer.jsx",
    ],
  },
  handler: {
    title: "HTTP Handler",
    what: "Точка входа API на бэкенде",
    does: "Принимает HTTP-запрос, вызывает Pipeline, возвращает JSON с formation",
    endpoints: [
      "POST /api/v1/pipeline → основной запрос",
      "POST /api/v1/expand → раскрытие карточки",
      "POST /api/v1/back → навигация назад",
    ],
    files: ["internal/handlers/"],
  },
  pipeline: {
    title: "Pipeline",
    what: "Оркестратор: запускает Agent1, потом Agent2",
    does: "Последовательно вызывает Agent1 (данные) → Agent2 (рендер). Собирает метрики: время, токены, стоимость",
    flow: "Agent1.Execute() → Agent2.Execute() → Response",
    important: "Agent2 ВСЕГДА запускается после Agent1, даже если Agent1 не вызвал тул",
    files: ["internal/usecases/pipeline_execute.go"],
  },
  agent1: {
    title: "Agent 1 — Данные",
    what: "LLM-агент для поиска данных в каталоге",
    does: 'Получает запрос пользователя. Если нужны новые данные — вызывает search_products. Если запрос про стиль — ничего не делает (stop_reason="end_turn")',
    tools: ["search_products — поиск товаров по каталогу"],
    not_tools: ["render_* и freestyle — НЕ видит (после фикса)"],
    data_in: "Текст пользователя + история разговора",
    data_out: "Товары записаны в State (через UpdateData)",
    files: [
      "internal/usecases/agent1_execute.go",
      "internal/prompts/prompt_analyze_query.go",
    ],
  },
  agent2: {
    title: "Agent 2 — Рендер",
    what: "LLM-агент для визуального оформления",
    does: "Берёт данные из State и решает КАК их показать. Выбирает пресет (product_grid, product_card...) или freestyle для кастомного стиля",
    tools: [
      "render_product_preset — стандартные шаблоны товаров",
      "render_service_preset — стандартные шаблоны услуг",
      "freestyle — кастомный стиль (большие заголовки, только фотки и т.д.)",
    ],
    decision:
      'Есть data_change → пресет. Нет data_change + стилевой запрос → freestyle',
    data_in: "productCount, fields, user_request, data_change (или null)",
    data_out: "Formation (шаблон + виджеты) записана в State",
    files: [
      "internal/usecases/agent2_execute.go",
      "internal/prompts/prompt_compose_widgets.go",
    ],
  },
  search_products: {
    title: "Тулл: search_products",
    what: "Поиск товаров в каталоге",
    does: "Запрашивает PostgreSQL через CatalogPort. Фильтрует по запросу, бренду, цене. Результат записывает в State",
    params: "query, category, brand, min_price, max_price, limit",
    result: 'Товары → State.data.products + Delta {path: "data.products"}',
    files: ["internal/tools/tool_search_products.go"],
  },
  render_preset: {
    title: "Тулл: render_*_preset",
    what: "Рендер товаров/услуг по стандартному шаблону",
    does: "Берёт товары из State, применяет пресет (grid/card/detail/compact), создаёт виджеты с атомами",
    presets: [
      "product_grid — сетка карточек",
      "product_card — одна карточка",
      "product_detail — подробная карточка",
      "product_compact — компактный список",
    ],
    result: "Formation → State.template + Delta",
    files: [
      "internal/tools/tool_render_preset.go",
      "internal/presets/",
    ],
  },
  freestyle: {
    title: "Тулл: freestyle",
    what: "Кастомное оформление",
    does: 'Когда стандартные пресеты не подходят. Позволяет указать стиль (product-hero), формацию (grid/list/carousel) и переопределить отображение слотов (title→h1, price→price-lg)',
    when: "Пользователь просит: крупнее, мельче, только фотки, hero-стиль и т.д.",
    params: "entity_type, formation, style (алиас), overrides (слот→display)",
    files: ["internal/tools/tool_freestyle.go"],
  },
  state: {
    title: "SessionState",
    what: "Полное состояние сессии пользователя",
    does: "Хранит всё: данные (товары), шаблон отображения, навигацию, историю разговора",
    zones: [
      "Data — товары и услуги (products[], services[])",
      "Meta — счётчики, поля, алиасы (productCount, fields...)",
      "Template — текущий шаблон отображения (Formation)",
      "View — режим просмотра (grid/detail) + focused entity",
      "ViewStack — стек навигации (для кнопки Назад)",
      "ConversationHistory — история разговора с LLM",
    ],
    important:
      "Каждое изменение = Zone Write + Delta. Атомарно в транзакции PostgreSQL",
    files: ["internal/domain/state_entity.go"],
  },
  delta: {
    title: "Delta (Событие)",
    what: "Запись о каждом изменении состояния",
    does: "Event Sourcing — можно воспроизвести любое состояние, откатиться к любому шагу",
    fields: [
      'step — порядковый номер (1, 2, 3...)',
      'trigger — что вызвало (USER_QUERY, WIDGET_ACTION)',
      'actorID — кто сделал (agent1, agent2, user_click)',
      'path — что изменилось (data.products, template.formation)',
      'action — детали ({tool: "search_products", params: {...}})',
    ],
    example:
      'Шаг 3: agent1 → search_products → data.products (добавил 12 товаров)',
    files: ["internal/domain/delta_entity.go"],
  },
  domain_entities: {
    title: "Доменные сущности",
    what: "Базовые строительные блоки UI",
    hierarchy: [
      "Atom — минимальный элемент (текст, число, картинка, иконка, видео, аудио)",
      "Widget — карточка из атомов (ProductCard, ServiceCard, ProductDetail...)",
      "Formation — раскладка виджетов (grid, list, carousel, single)",
    ],
    atom_slots: [
      "hero — главное изображение/карусель",
      "badge — бейдж-оверлей (статус, скидка)",
      "title — заголовок товара/услуги",
      "primary — основные атрибуты (показываются сразу)",
      "price — блок цены",
      "secondary — дополнительные атрибуты (раскрываемые)",
      "gallery — полная галерея (не только hero)",
      "stock — индикатор наличия",
      "description — полное описание",
      "tags — теги/чипсы",
      "specs — таблица характеристик",
    ],
    atom_displays: [
      "Текст: h1, h2, h3, h4, body-lg, body, body-sm, caption",
      "Бейджи: badge, badge-success, badge-error, badge-warning, tag, tag-active",
      "Цены: price, price-lg, price-old, price-discount",
      "Рейтинг: rating, rating-text, rating-compact",
      "Числа: percent, progress",
      "Картинки: image, image-cover, avatar, avatar-sm, avatar-lg, thumbnail, gallery",
      "Иконки: icon, icon-sm, icon-lg",
      "Кнопки: button-primary, button-secondary, button-outline, button-ghost, input",
      "Лейаут: divider, spacer",
    ],
  },
  postgres: {
    title: "PostgreSQL (Neon)",
    what: "Основная база данных",
    does: "Хранит состояние сессий, дельты, каталог товаров, сессии чата",
    tables: [
      "session_state — состояние (zones: data, meta, template, view)",
      "deltas — история изменений",
      "products — каталог товаров",
      "categories, brands — справочники",
      "tenants — мультитенантность",
    ],
    files: ["internal/adapters/postgres/"],
  },
  anthropic: {
    title: "Anthropic Claude API",
    what: "LLM-провайдер",
    does: "Agent1 и Agent2 общаются с Claude Haiku 4.5. Поддержка промпт-кэширования для экономии",
    models: ["Claude Haiku 4.5 — основная модель (быстрая, дешёвая)"],
    caching:
      "System prompt + tools + история кэшируются. Кэш живёт 5 минут. Экономия ~90% на input токенах",
    files: ["internal/adapters/anthropic/"],
  },
};

const Arrow = ({ from, to, label, color = COLORS.arrow }) => {
  return (
    <div className="flex items-center justify-center my-1">
      <div className="flex flex-col items-center">
        <svg width="24" height="28" viewBox="0 0 24 28">
          <line
            x1="12"
            y1="0"
            x2="12"
            y2="20"
            stroke={color}
            strokeWidth="2"
          />
          <polygon points="6,18 12,28 18,18" fill={color} />
        </svg>
        {label && (
          <span
            className="text-xs mt-0.5 px-2 py-0.5 rounded"
            style={{ color: COLORS.domain.text, background: "#F1F5F9" }}
          >
            {label}
          </span>
        )}
      </div>
    </div>
  );
};

const Node = ({ id, label, sublabel, color, onClick, isSelected, size = "md" }) => {
  const sizes = {
    sm: "px-3 py-1.5 text-sm",
    md: "px-4 py-2.5 text-sm",
    lg: "px-5 py-3 text-base",
  };
  return (
    <button
      onClick={() => onClick(id)}
      className={`rounded-lg font-medium transition-all duration-200 cursor-pointer border-2 ${sizes[size]}`}
      style={{
        background: isSelected ? color.border : color.bg,
        borderColor: color.border,
        color: isSelected ? "#fff" : color.text,
        boxShadow: isSelected ? `0 0 0 3px ${color.border}33` : "none",
        minWidth: size === "sm" ? 120 : 160,
      }}
    >
      <div className="font-semibold">{label}</div>
      {sublabel && (
        <div
          className="text-xs mt-0.5 opacity-75"
          style={{ color: isSelected ? "#ffffffcc" : color.text + "99" }}
        >
          {sublabel}
        </div>
      )}
    </button>
  );
};

const DetailPanel = ({ detail, onClose }) => {
  if (!detail) return null;
  const d = details[detail];
  if (!d) return null;

  return (
    <div
      className="rounded-xl p-5 border"
      style={{
        background: "#FAFBFC",
        borderColor: "#E2E8F0",
        maxHeight: 520,
        overflowY: "auto",
      }}
    >
      <div className="flex justify-between items-start mb-3">
        <h3 className="text-lg font-bold" style={{ color: "#1E293B" }}>
          {d.title}
        </h3>
        <button
          onClick={onClose}
          className="text-gray-400 hover:text-gray-600 text-xl leading-none cursor-pointer"
        >
          x
        </button>
      </div>

      <div className="space-y-3">
        {d.what && (
          <div>
            <span className="text-xs font-semibold uppercase tracking-wide text-gray-400">
              Что это
            </span>
            <p className="text-sm text-gray-700 mt-0.5">{d.what}</p>
          </div>
        )}
        {d.does && (
          <div>
            <span className="text-xs font-semibold uppercase tracking-wide text-gray-400">
              Что делает
            </span>
            <p className="text-sm text-gray-700 mt-0.5">{d.does}</p>
          </div>
        )}
        {d.important && (
          <div
            className="rounded-lg p-3 text-sm"
            style={{ background: "#FEF3C7", color: "#92400E" }}
          >
            {d.important}
          </div>
        )}
        {d.decision && (
          <div
            className="rounded-lg p-3 text-sm"
            style={{ background: "#EDE9FE", color: "#5B21B6" }}
          >
            {d.decision}
          </div>
        )}

        {d.tools && (
          <div>
            <span className="text-xs font-semibold uppercase tracking-wide text-gray-400">
              Тулы
            </span>
            <ul className="mt-1 space-y-1">
              {d.tools.map((t, i) => (
                <li key={i} className="text-sm text-gray-700 flex items-start">
                  <span className="text-green-500 mr-2 mt-0.5">+</span>
                  {t}
                </li>
              ))}
            </ul>
          </div>
        )}
        {d.not_tools && (
          <ul className="space-y-1">
            {d.not_tools.map((t, i) => (
              <li key={i} className="text-sm text-gray-700 flex items-start">
                <span className="text-red-400 mr-2 mt-0.5">x</span>
                {t}
              </li>
            ))}
          </ul>
        )}

        {d.zones && (
          <div>
            <span className="text-xs font-semibold uppercase tracking-wide text-gray-400">
              Зоны данных
            </span>
            <ul className="mt-1 space-y-1">
              {d.zones.map((z, i) => (
                <li key={i} className="text-sm text-gray-700">
                  <span className="text-rose-400 mr-1">&bull;</span> {z}
                </li>
              ))}
            </ul>
          </div>
        )}

        {d.hierarchy && (
          <div>
            <span className="text-xs font-semibold uppercase tracking-wide text-gray-400">
              Иерархия
            </span>
            <ul className="mt-1 space-y-1">
              {d.hierarchy.map((h, i) => (
                <li key={i} className="text-sm text-gray-700">
                  {"  ".repeat(i)}
                  {i > 0 ? "└ " : ""}
                  {h}
                </li>
              ))}
            </ul>
          </div>
        )}

        {d.atom_slots && (
          <div>
            <span className="text-xs font-semibold uppercase tracking-wide text-gray-400">
              Слоты атомов (11)
            </span>
            <ul className="mt-1 space-y-1">
              {d.atom_slots.map((s, i) => (
                <li key={i} className="text-sm text-gray-700">
                  <span className="text-blue-400 mr-1">&bull;</span> {s}
                </li>
              ))}
            </ul>
          </div>
        )}
        {d.atom_displays && (
          <div>
            <span className="text-xs font-semibold uppercase tracking-wide text-gray-400">
              Все Display-значения
            </span>
            <ul className="mt-1 space-y-1">
              {d.atom_displays.map((d2, i) => (
                <li key={i} className="text-xs font-mono text-gray-600">
                  {d2}
                </li>
              ))}
            </ul>
          </div>
        )}

        {d.presets && (
          <div>
            <span className="text-xs font-semibold uppercase tracking-wide text-gray-400">
              Пресеты
            </span>
            <ul className="mt-1 space-y-1">
              {d.presets.map((p, i) => (
                <li key={i} className="text-sm text-gray-700">
                  {p}
                </li>
              ))}
            </ul>
          </div>
        )}

        {d.fields && (
          <div>
            <span className="text-xs font-semibold uppercase tracking-wide text-gray-400">
              Поля
            </span>
            <ul className="mt-1 space-y-1">
              {d.fields.map((f, i) => (
                <li key={i} className="text-sm text-gray-700">
                  {f}
                </li>
              ))}
            </ul>
          </div>
        )}

        {d.endpoints && (
          <div>
            <span className="text-xs font-semibold uppercase tracking-wide text-gray-400">
              Эндпоинты
            </span>
            <ul className="mt-1 space-y-1">
              {d.endpoints.map((e, i) => (
                <li
                  key={i}
                  className="text-sm font-mono"
                  style={{ color: "#B45309" }}
                >
                  {e}
                </li>
              ))}
            </ul>
          </div>
        )}

        {d.examples && (
          <div>
            <span className="text-xs font-semibold uppercase tracking-wide text-gray-400">
              Примеры
            </span>
            <ul className="mt-1 space-y-1">
              {d.examples.map((e, i) => (
                <li key={i} className="text-sm text-gray-600 italic">
                  {e}
                </li>
              ))}
            </ul>
          </div>
        )}

        {d.tables && (
          <div>
            <span className="text-xs font-semibold uppercase tracking-wide text-gray-400">
              Таблицы
            </span>
            <ul className="mt-1 space-y-1">
              {d.tables.map((t, i) => (
                <li key={i} className="text-sm text-gray-700">
                  {t}
                </li>
              ))}
            </ul>
          </div>
        )}

        {d.data_in && (
          <div>
            <span className="text-xs font-semibold uppercase tracking-wide text-gray-400">
              Входные данные
            </span>
            <p className="text-sm text-gray-700 mt-0.5 font-mono text-xs">
              {d.data_in}
            </p>
          </div>
        )}
        {d.data_out && (
          <div>
            <span className="text-xs font-semibold uppercase tracking-wide text-gray-400">
              Выходные данные
            </span>
            <p className="text-sm text-gray-700 mt-0.5 font-mono text-xs">
              {d.data_out}
            </p>
          </div>
        )}

        {d.files && (
          <div>
            <span className="text-xs font-semibold uppercase tracking-wide text-gray-400">
              Файлы
            </span>
            <ul className="mt-1">
              {d.files.map((f, i) => (
                <li
                  key={i}
                  className="text-xs font-mono"
                  style={{ color: "#64748B" }}
                >
                  {f}
                </li>
              ))}
            </ul>
          </div>
        )}
      </div>
    </div>
  );
};

const scenarios = [
  {
    id: "search",
    title: "Поиск товаров",
    query: '"покажи кроссовки Nike"',
    steps: [
      { actor: "user", action: "Пишет запрос в чат", color: COLORS.user },
      { actor: "chatpanel", action: 'POST /api/v1/pipeline { query: "кроссовки Nike" }', color: COLORS.frontend },
      { actor: "agent1", action: "LLM решает: нужны данные → вызывает search_products", color: COLORS.agent1 },
      { actor: "search", action: "Ищет в каталоге → находит 12 товаров → пишет в State", color: COLORS.agent1 },
      { actor: "agent2", action: "Видит 12 товаров + data_change → render_product_preset(product_grid)", color: COLORS.agent2 },
      { actor: "render", action: "Строит 12 виджетов с атомами (фото, название, цена, рейтинг)", color: COLORS.agent2 },
      { actor: "frontend", action: "Рендерит сетку 3x4 с карточками товаров", color: COLORS.frontend },
    ],
  },
  {
    id: "style",
    title: "Смена стиля",
    query: '"покажи с большими заголовками"',
    steps: [
      { actor: "user", action: "Просит изменить отображение", color: COLORS.user },
      { actor: "agent1", action: "LLM решает: данные не нужны → НЕ вызывает тул → stop", color: COLORS.agent1 },
      { actor: "agent2", action: "Видит data_change=null + стилевой запрос → freestyle", color: COLORS.agent2 },
      { actor: "freestyle", action: 'freestyle(style="product-hero", formation="grid") → h1 заголовки, крупные цены', color: COLORS.agent2 },
      { actor: "frontend", action: "Те же 12 товаров, но с крупными заголовками", color: COLORS.frontend },
    ],
  },
  {
    id: "expand",
    title: "Навигация: детали",
    query: "Клик на карточку товара",
    steps: [
      { actor: "user", action: "Кликает на карточку Nike Air Max", color: COLORS.user },
      { actor: "frontend", action: "POST /api/v1/expand { entityId: nike_air_max }", color: COLORS.frontend },
      { actor: "state", action: "Текущий вид → ViewStack (сохраняем для возврата)", color: COLORS.state },
      { actor: "view", action: 'View.mode = "detail", View.focused = nike_air_max', color: COLORS.state },
      { actor: "frontend", action: "Одна большая карточка с галереей, описанием, спеками. Кнопка Назад", color: COLORS.frontend },
    ],
  },
];

const ScenarioView = ({ scenario }) => {
  return (
    <div className="space-y-2">
      <div
        className="text-sm font-mono px-3 py-1.5 rounded-lg inline-block"
        style={{ background: "#F1F5F9", color: "#475569" }}
      >
        {scenario.query}
      </div>
      <div className="space-y-1.5">
        {scenario.steps.map((step, i) => (
          <div key={i} className="flex items-start gap-2">
            <div
              className="w-5 h-5 rounded-full flex items-center justify-center text-xs font-bold shrink-0 mt-0.5"
              style={{ background: step.color.border, color: "#fff" }}
            >
              {i + 1}
            </div>
            <p className="text-sm" style={{ color: "#334155" }}>
              {step.action}
            </p>
          </div>
        ))}
      </div>
    </div>
  );
};

export default function ArchitectureMap() {
  const [selected, setSelected] = useState(null);
  const [activeTab, setActiveTab] = useState("pipeline");
  const [activeScenario, setActiveScenario] = useState("search");

  const handleClick = (id) => {
    setSelected(selected === id ? null : id);
  };

  return (
    <div
      className="min-h-screen p-4"
      style={{ background: "#FFFFFF", fontFamily: "system-ui, -apple-system, sans-serif" }}
    >
      <div className="max-w-5xl mx-auto">
        <h1
          className="text-2xl font-bold mb-1"
          style={{ color: "#0F172A" }}
        >
          Keepstar — Карта архитектуры
        </h1>
        <p className="text-sm mb-5" style={{ color: "#64748B" }}>
          Кликни на любой элемент чтобы увидеть детали
        </p>

        {/* Tabs */}
        <div className="flex gap-1 mb-5 p-1 rounded-lg" style={{ background: "#F1F5F9" }}>
          {[
            { id: "pipeline", label: "Pipeline" },
            { id: "entities", label: "Сущности" },
            { id: "scenarios", label: "Сценарии" },
            { id: "state", label: "State" },
          ].map((tab) => (
            <button
              key={tab.id}
              onClick={() => { setActiveTab(tab.id); setSelected(null); }}
              className={`px-4 py-2 rounded-md text-sm font-medium transition-all cursor-pointer ${
                activeTab === tab.id ? "shadow-sm" : ""
              }`}
              style={{
                background: activeTab === tab.id ? "#fff" : "transparent",
                color: activeTab === tab.id ? "#0F172A" : "#64748B",
              }}
            >
              {tab.label}
            </button>
          ))}
        </div>

        <div className="grid grid-cols-1 lg:grid-cols-5 gap-5">
          {/* Main diagram */}
          <div className="lg:col-span-3">
            {/* ===== PIPELINE TAB ===== */}
            {activeTab === "pipeline" && (
              <div className="space-y-2">
                {/* Entry point */}
                <div className="flex items-center gap-2">
                  <Node id="user" label="Пользователь" sublabel="текст / клик" color={COLORS.user} onClick={handleClick} isSelected={selected === "user"} />
                  <span style={{color: COLORS.arrow}}>→</span>
                  <Node id="chatpanel" label="Frontend" sublabel="ChatPanel" color={COLORS.frontend} onClick={handleClick} isSelected={selected === "chatpanel"} size="sm" />
                </div>

                {/* Route decision */}
                <div className="rounded-xl p-3 border-2" style={{ borderColor: COLORS.handler.border, background: COLORS.handler.bg }}>
                  <div className="text-sm font-bold mb-2 cursor-pointer" style={{ color: COLORS.handler.text }} onClick={() => handleClick("handler")}>
                    HTTP Router — какой запрос пришёл?
                  </div>
                  <div className="flex gap-2 flex-wrap">
                    {[
                      { label: "POST /pipeline", desc: "текстовый запрос → Оркестратор", active: true },
                      { label: "POST /expand", desc: "клик на карточку → напрямую в State" },
                      { label: "POST /back", desc: "кнопка назад → напрямую в State" },
                    ].map((r) => (
                      <div key={r.label} className="px-3 py-1.5 rounded-lg text-xs border" style={{
                        borderColor: r.active ? COLORS.handler.border : "#E2E8F0",
                        background: r.active ? "#fff" : "#FAFBFC",
                        fontWeight: r.active ? 600 : 400,
                      }}>
                        <div style={{ color: COLORS.handler.text }}>{r.label}</div>
                        <div className="text-gray-400">{r.desc}</div>
                      </div>
                    ))}
                  </div>
                </div>

                {/* ===== ORCHESTRATOR ===== */}
                <div className="rounded-xl p-4 border-2" style={{ borderColor: COLORS.handler.border, background: "#FFFDF5" }}>
                  <div className="text-sm font-bold mb-1 cursor-pointer" style={{ color: COLORS.handler.text }} onClick={() => handleClick("pipeline")}>
                    Оркестратор — Pipeline.Execute()
                  </div>
                  <div className="text-xs text-gray-400 mb-3">pipeline_execute.go — управляет всем потоком, сам ничего не делает с данными</div>

                  <div className="space-y-1.5">
                    <div className="flex items-start gap-2">
                      <div className="w-4 h-4 rounded-full flex items-center justify-center text-xs font-bold shrink-0 mt-0.5" style={{ background: COLORS.handler.border, color: "#fff" }}>1</div>
                      <div className="text-xs">
                        <span className="font-semibold">Сессия:</span>
                        <span className="text-gray-500"> Проверяет cachePort.GetSession(). Если ErrSessionNotFound → создаёт Session{'{'}status: active{'}'} и сохраняет. Нужно для FK-constraint в таблице state</span>
                      </div>
                    </div>
                    <div className="flex items-start gap-2">
                      <div className="w-4 h-4 rounded-full flex items-center justify-center text-xs font-bold shrink-0 mt-0.5" style={{ background: COLORS.handler.border, color: "#fff" }}>2</div>
                      <div className="text-xs">
                        <span className="font-semibold">TurnID:</span>
                        <span className="text-gray-500"> Генерирует uuid. Все дельты этого запроса будут иметь один TurnID — для группировки "что произошло за один ход"</span>
                      </div>
                    </div>
                    <div className="flex items-start gap-2">
                      <div className="w-4 h-4 rounded-full flex items-center justify-center text-xs font-bold shrink-0 mt-0.5" style={{ background: COLORS.handler.border, color: "#fff" }}>3</div>
                      <div className="text-xs">
                        <span className="font-semibold">Agent1.Execute():</span>
                        <span className="text-gray-500"> Передаёт {'{'}sessionID, query, tenantSlug, turnID{'}'}. Ждёт завершения. Если ошибка → return error, pipeline останавливается</span>
                      </div>
                    </div>
                    <div className="flex items-start gap-2">
                      <div className="w-4 h-4 rounded-full flex items-center justify-center text-xs font-bold shrink-0 mt-0.5" style={{ background: COLORS.handler.border, color: "#fff" }}>4</div>
                      <div className="text-xs">
                        <span className="font-semibold">Agent2.Execute():</span>
                        <span className="text-gray-500"> Передаёт {'{'}sessionID, turnID, userQuery{'}'}. Запускается ВСЕГДА, даже если Agent1 не вызвал тул</span>
                      </div>
                    </div>
                    <div className="flex items-start gap-2">
                      <div className="w-4 h-4 rounded-full flex items-center justify-center text-xs font-bold shrink-0 mt-0.5" style={{ background: COLORS.handler.border, color: "#fff" }}>5</div>
                      <div className="text-xs">
                        <span className="font-semibold">Сборка Formation:</span>
                        <span className="text-gray-500"> Три fallback-а: 1) agent2Resp.Formation → 2) state.Template["formation"] из БД → 3) legacy ApplyTemplate. Первый найденный выигрывает</span>
                      </div>
                    </div>
                    <div className="flex items-start gap-2">
                      <div className="w-4 h-4 rounded-full flex items-center justify-center text-xs font-bold shrink-0 mt-0.5" style={{ background: COLORS.handler.border, color: "#fff" }}>6</div>
                      <div className="text-xs">
                        <span className="font-semibold">Ответ:</span>
                        <span className="text-gray-500"> Собирает PipelineExecuteResponse: formation + метрики обоих агентов (ms, tokens, cost, tool called, products found)</span>
                      </div>
                    </div>
                  </div>

                  <div className="mt-3 p-2 rounded-lg text-xs" style={{ background: "#FEF3C7", color: "#92400E" }}>
                    Оркестратор не меняет State, не вызывает LLM, не выбирает тулы. Он только: создаёт сессию, генерирует TurnID, вызывает агентов последовательно, собирает ответ. Вся логика — в агентах.
                  </div>
                </div>

                {/* ===== THREE LAYERS ===== */}
                <div className="text-xs text-center font-semibold py-1" style={{ color: "#64748B" }}>▼ Три слоя архитектуры ▼</div>

                {/* ===== LAYER 1: INPUT ===== */}
                <div className="rounded-xl border-2 overflow-hidden" style={{ borderColor: COLORS.agent1.border }}>
                  <div className="px-4 py-2 flex items-center justify-between" style={{ background: COLORS.agent1.border }}>
                    <span className="text-sm font-bold text-white">Слой 1 — Вход / Маршрутизатор</span>
                    <span className="text-xs text-white opacity-75">Agent 1 · agent1_execute.go</span>
                  </div>
                  <div className="p-4 cursor-pointer" style={{ background: COLORS.agent1.bg }} onClick={() => handleClick("agent1")}>
                    <div className="text-xs text-gray-500 mb-3">
                      Принимает действие пользователя. Решает: нужны ли новые данные? Если да — вызывает search-тулы и пишет в State. Если нет — пропускает (pipeline продолжает к Слою 3).
                    </div>

                    <div className="space-y-2">
                      <div className="flex items-start gap-2">
                        <div className="w-5 h-5 rounded-full flex items-center justify-center text-xs font-bold shrink-0" style={{ background: COLORS.agent1.border, color: "#fff" }}>1</div>
                        <div className="text-xs">
                          <span className="font-semibold">GetState / CreateState</span>
                          <span className="text-gray-500"> — читает текущий state. Если первый запрос — создаёт пустой. Ставит tenant_slug.</span>
                        </div>
                      </div>

                      <div className="flex items-start gap-2">
                        <div className="w-5 h-5 rounded-full flex items-center justify-center text-xs font-bold shrink-0" style={{ background: COLORS.agent1.border, color: "#fff" }}>2</div>
                        <div className="text-xs">
                          <span className="font-semibold">ConversationHistory + user message</span>
                          <span className="text-gray-500"> — если history {'>'} 1 → prompt caching включён</span>
                        </div>
                      </div>

                      <div className="flex items-start gap-2">
                        <div className="w-5 h-5 rounded-full flex items-center justify-center text-xs font-bold shrink-0" style={{ background: COLORS.agent1.border, color: "#fff" }}>3</div>
                        <div className="text-xs">
                          <span className="font-semibold">LLM с тулами (только search_*)</span>
                        </div>
                      </div>

                      {/* Branch */}
                      <div className="ml-7 rounded-lg p-2.5 border" style={{ borderColor: COLORS.agent1.border + "44", background: "#fff" }}>
                        <div className="grid grid-cols-2 gap-2">
                          <div className="rounded-lg p-2 border" style={{ borderColor: "#22C55E44", background: "#F0FDF4" }}>
                            <div className="text-xs font-bold" style={{ color: "#15803D" }}>tool_use → search_products</div>
                            <div className="text-xs text-gray-500 mt-1">
                              → CatalogPort.ListProducts()
                              <br/>→ <span className="font-mono text-rose-500">UpdateData</span> пишет в State.Data + State.Meta + Delta
                              <br/>→ toolResult = "ok" / "empty"
                            </div>
                          </div>
                          <div className="rounded-lg p-2 border" style={{ borderColor: "#F59E0B44", background: "#FFFBEB" }}>
                            <div className="text-xs font-bold" style={{ color: "#B45309" }}>end_turn → ничего</div>
                            <div className="text-xs text-gray-500 mt-1">
                              → warn "no_tool_call"
                              <br/>→ State не тронут
                              <br/>→ <span className="font-semibold">Слой 3 всё равно запустится</span>
                            </div>
                          </div>
                        </div>
                      </div>

                      <div className="flex items-start gap-2">
                        <div className="w-5 h-5 rounded-full flex items-center justify-center text-xs font-bold shrink-0" style={{ background: COLORS.agent1.border, color: "#fff" }}>4</div>
                        <div className="text-xs">
                          <span className="font-semibold">AppendConversation</span>
                          <span className="text-gray-500"> — всегда +user. Если тул был: +assistant:tool_use +user:tool_result</span>
                        </div>
                      </div>
                    </div>

                    <div className="mt-3 flex gap-2 text-xs">
                      <span className="px-2 py-0.5 rounded" style={{ background: "#fff", color: COLORS.agent1.text, border: `1px solid ${COLORS.agent1.border}44` }}>Пишет: Data, Meta, ConversationHistory</span>
                      <span className="px-2 py-0.5 rounded" style={{ background: "#fff", color: "#94A3B8" }}>Не трогает: Template, View, ViewStack</span>
                    </div>
                  </div>
                </div>

                {/* ===== LAYER 2: STATE ===== */}
                <div className="rounded-xl border-2 overflow-hidden" style={{ borderColor: COLORS.state.border }}>
                  <div className="px-4 py-2 flex items-center justify-between" style={{ background: COLORS.state.border }}>
                    <span className="text-sm font-bold text-white">Слой 2 — State (6 контейнеров)</span>
                    <span className="text-xs text-white opacity-75">SessionState · state_entity.go</span>
                  </div>
                  <div className="p-4" style={{ background: COLORS.state.bg }} onClick={() => handleClick("state")}>
                    <div className="grid grid-cols-3 gap-2">
                      {[
                        { name: "Data", desc: "products[], services[]", writer: "Слой 1", color: COLORS.agent1.border },
                        { name: "Meta", desc: "count, fields, aliases", writer: "Слой 1", color: COLORS.agent1.border },
                        { name: "Template", desc: "formation (виджеты)", writer: "Слой 3", color: COLORS.agent2.border },
                        { name: "View", desc: "mode, focused", writer: "expand/back", color: COLORS.state.border },
                        { name: "ViewStack", desc: "навигация []snapshot", writer: "expand/back", color: COLORS.state.border },
                        { name: "ConversationHistory", desc: "LLM-история", writer: "Слой 1", color: COLORS.agent1.border },
                      ].map((c) => (
                        <div key={c.name} className="rounded-lg p-2 bg-white border-l-3" style={{ borderLeft: `3px solid ${c.color}` }}>
                          <div className="text-xs font-bold" style={{ color: COLORS.state.text }}>{c.name}</div>
                          <div className="text-xs text-gray-500">{c.desc}</div>
                          <div className="text-xs mt-1" style={{ color: c.color }}>← {c.writer}</div>
                        </div>
                      ))}
                    </div>
                    <div className="mt-3 p-2 rounded-lg text-xs" style={{ background: "#fff", color: "#64748B" }}>
                      Каждая запись = zone-write (атомарно в PostgreSQL транзакции) + Delta (event sourcing). Дельты хранят полную историю: кто, когда, что изменил. Step увеличивается на каждое изменение.
                    </div>
                  </div>
                </div>

                {/* ===== LAYER 3: OUTPUT ===== */}
                <div className="rounded-xl border-2 overflow-hidden" style={{ borderColor: COLORS.agent2.border }}>
                  <div className="px-4 py-2 flex items-center justify-between" style={{ background: COLORS.agent2.border }}>
                    <span className="text-sm font-bold text-white">Слой 3 — Выход / Рендер</span>
                    <span className="text-xs text-white opacity-75">Agent 2 · agent2_execute.go</span>
                  </div>
                  <div className="p-4 cursor-pointer" style={{ background: COLORS.agent2.bg + "44" }} onClick={() => handleClick("agent2")}>
                    <div className="text-xs text-gray-500 mb-3">
                      Читает данные из State. Решает КАК их показать. Пишет Formation (виджеты + раскладку) обратно в State.Template.
                    </div>

                    <div className="space-y-2">
                      <div className="flex items-start gap-2">
                        <div className="w-5 h-5 rounded-full flex items-center justify-center text-xs font-bold shrink-0" style={{ background: COLORS.agent2.border, color: "#fff" }}>1</div>
                        <div className="text-xs">
                          <span className="font-semibold">Читает State.Data → подсчёт</span>
                          <span className="text-gray-500"> productCount, serviceCount</span>
                        </div>
                      </div>

                      {/* Short circuit */}
                      <div className="ml-7 rounded-lg p-1.5 border" style={{ borderColor: "#F43F5E44", background: "#FFF1F2" }}>
                        <div className="text-xs">
                          <span className="font-bold" style={{ color: "#BE123C" }}>0 products И 0 services → return nil</span>
                          <span className="text-gray-500"> (нечего рендерить)</span>
                        </div>
                      </div>

                      <div className="flex items-start gap-2">
                        <div className="w-5 h-5 rounded-full flex items-center justify-center text-xs font-bold shrink-0" style={{ background: COLORS.agent2.border, color: "#fff" }}>2</div>
                        <div className="text-xs">
                          <span className="font-semibold">Ищет dataDelta текущего TurnID</span>
                          <span className="text-gray-500"> — была ли запись в data.* в этом ходе? Если Слой 1 вызвал тул → dataDelta есть. Если нет → null.</span>
                        </div>
                      </div>

                      <div className="flex items-start gap-2">
                        <div className="w-5 h-5 rounded-full flex items-center justify-center text-xs font-bold shrink-0" style={{ background: COLORS.agent2.border, color: "#fff" }}>3</div>
                        <div className="text-xs">
                          <span className="font-semibold">Промпт для LLM</span>
                          <span className="text-gray-500"> — JSON: {'{'}productCount, fields, view_mode, user_request, data_change: {'{...}'} или null{'}'}</span>
                        </div>
                      </div>

                      <div className="flex items-start gap-2">
                        <div className="w-5 h-5 rounded-full flex items-center justify-center text-xs font-bold shrink-0" style={{ background: COLORS.agent2.border, color: "#fff" }}>4</div>
                        <div className="text-xs"><span className="font-semibold">LLM выбирает тул:</span></div>
                      </div>

                      {/* Branch */}
                      <div className="ml-7 rounded-lg p-2.5 border" style={{ borderColor: COLORS.agent2.border + "44", background: "#fff" }}>
                        <div className="space-y-2">
                          <div className="rounded-lg p-2 border" style={{ borderColor: "#22C55E44", background: "#F0FDF4" }}>
                            <div className="text-xs font-bold" style={{ color: "#15803D" }}>render_*_preset</div>
                            <div className="text-xs text-gray-500">
                              Когда: data_change есть ИЛИ нет user_request
                              <br/>→ Пресет по количеству (1→card, 2-6→grid, 7+→grid/compact)
                              <br/>→ Строит Widget[]{'{'}template: ProductCard, atoms: [...]{'}'} из products
                              <br/>→ <span className="font-mono text-rose-500">UpdateTemplate</span> пишет в State.Template + Delta
                            </div>
                          </div>
                          <div className="rounded-lg p-2 border" style={{ borderColor: "#8B5CF644", background: "#F5F3FF" }}>
                            <div className="text-xs font-bold" style={{ color: "#6D28D9" }}>freestyle</div>
                            <div className="text-xs text-gray-500">
                              Когда: data_change = null + стилевой user_request
                              <br/>→ Стиль-алиас или overrides (slot→display)
                              <br/>→ Строит Widget[]{'{'}template: ProductCard, atoms с другими display{'}'}
                              <br/>→ <span className="font-mono text-rose-500">UpdateTemplate</span> пишет в State.Template + Delta
                              <br/><span className="font-semibold text-amber-600">Факт: те же Widget/ProductCard, только display атомов другой</span>
                            </div>
                          </div>
                        </div>
                      </div>

                      <div className="flex items-start gap-2">
                        <div className="w-5 h-5 rounded-full flex items-center justify-center text-xs font-bold shrink-0" style={{ background: COLORS.agent2.border, color: "#fff" }}>5</div>
                        <div className="text-xs">
                          <span className="font-semibold">Читает Formation из State.Template["formation"]</span>
                          <span className="text-gray-500"> → возвращает оркестратору</span>
                        </div>
                      </div>
                    </div>

                    <div className="mt-3 flex gap-2 text-xs">
                      <span className="px-2 py-0.5 rounded" style={{ background: "#fff", color: COLORS.agent2.text, border: `1px solid ${COLORS.agent2.border}44` }}>Пишет: Template</span>
                      <span className="px-2 py-0.5 rounded" style={{ background: "#fff", color: COLORS.agent1.text, border: `1px solid ${COLORS.agent1.border}44` }}>Читает: Data, Meta</span>
                      <span className="px-2 py-0.5 rounded" style={{ background: "#fff", color: "#94A3B8" }}>Не трогает: View, ViewStack, ConversationHistory</span>
                    </div>
                  </div>
                </div>

                {/* Response to frontend */}
                <div className="rounded-xl p-3 border cursor-pointer" style={{ borderColor: COLORS.frontend.border, background: COLORS.frontend.bg }} onClick={() => handleClick("formation_renderer")}>
                  <div className="text-sm font-bold" style={{ color: COLORS.frontend.text }}>
                    → Frontend получает JSON
                  </div>
                  <div className="text-xs font-mono text-gray-500 mt-1">
                    {'{ sessionId, formation: { mode, grid, widgets[] }, agent1Ms, agent2Ms, totalMs }'}
                  </div>
                </div>

                {/* Expand/Back paths */}
                <div className="rounded-xl p-4 border-2" style={{ borderColor: "#E2E8F0" }}>
                  <div className="text-sm font-bold mb-1" style={{ color: COLORS.state.text }}>
                    Быстрые пути (без LLM, мимо Слоёв 1 и 3)
                  </div>
                  <div className="text-xs text-gray-400 mb-3">
                    Пользователь кликает кнопку → Handler идёт напрямую в State → возвращает Formation фронту. State всё равно пишет дельты (для истории и контекста).
                  </div>
                  <div className="grid grid-cols-2 gap-3">
                    <div className="rounded-lg p-3 border" style={{ borderColor: COLORS.state.border + "44", background: COLORS.state.bg }}>
                      <div className="text-xs font-bold" style={{ color: COLORS.state.text }}>POST /expand (клик на карточку)</div>
                      <div className="text-xs text-gray-500 mt-1 space-y-0.5">
                        <div>1. Текущий вид → <span className="font-mono text-rose-500">PushView</span> в ViewStack + Delta</div>
                        <div>2. View.mode = "detail", View.focused = entity</div>
                        <div>3. Строит formation для 1 товара (detail preset)</div>
                        <div>4. → Frontend: formation + canGoBack=true</div>
                      </div>
                    </div>
                    <div className="rounded-lg p-3 border" style={{ borderColor: COLORS.state.border + "44", background: COLORS.state.bg }}>
                      <div className="text-xs font-bold" style={{ color: COLORS.state.text }}>POST /back (кнопка назад)</div>
                      <div className="text-xs text-gray-500 mt-1 space-y-0.5">
                        <div>1. <span className="font-mono text-rose-500">PopView</span> ← ViewStack + Delta</div>
                        <div>2. Восстановить View из snapshot</div>
                        <div>3. Восстановить formation предыдущего шага</div>
                        <div>4. → Frontend: formation + canGoBack=(stack{'>'} 0)</div>
                      </div>
                    </div>
                  </div>
                </div>

                {/* External services */}
                <div className="flex gap-4 mt-3 w-full justify-center">
                  <Node id="postgres" label="PostgreSQL" sublabel="State + Каталог + Deltas" color={COLORS.db} onClick={handleClick} isSelected={selected === "postgres"} size="sm" />
                  <Node id="anthropic" label="Claude Haiku" sublabel="Слой 1 + Слой 3" color={COLORS.llm} onClick={handleClick} isSelected={selected === "anthropic"} size="sm" />
                </div>
              </div>
            )}

            {/* ===== ENTITIES TAB ===== */}
            {activeTab === "entities" && (
              <div className="flex flex-col items-center gap-2">
                <p className="text-sm text-gray-500 mb-3">
                  Как строится UI: от данных к экрану
                </p>

                {/* Product/Service */}
                <div
                  className="rounded-xl p-4 w-full border"
                  style={{ borderColor: COLORS.db.border, background: COLORS.db.bg }}
                >
                  <div className="text-sm font-bold mb-2" style={{ color: COLORS.db.text }}>
                    Каталог (PostgreSQL)
                  </div>
                  <div className="flex gap-3 flex-wrap">
                    <div className="px-3 py-2 rounded-lg bg-white border text-sm" style={{ borderColor: COLORS.db.border }}>
                      <div className="font-semibold">Product</div>
                      <div className="text-xs text-gray-500">name, price, images, rating, brand, category, tags</div>
                    </div>
                    <div className="px-3 py-2 rounded-lg bg-white border text-sm" style={{ borderColor: COLORS.db.border }}>
                      <div className="font-semibold">Service</div>
                      <div className="text-xs text-gray-500">name, price, duration, provider, images</div>
                    </div>
                  </div>
                </div>

                <Arrow label="search_products → State.data" />

                {/* Atoms */}
                <div
                  className="rounded-xl p-4 w-full border cursor-pointer"
                  style={{ borderColor: COLORS.domain.border, background: "#fff" }}
                  onClick={() => handleClick("domain_entities")}
                >
                  <div className="text-sm font-bold mb-2" style={{ color: COLORS.domain.text }}>
                    Atom — минимальный элемент (6 типов: text, number, image, icon, video, audio)
                  </div>
                  <div className="text-xs text-gray-400 mb-2">11 слотов — кликни для подробностей</div>
                  <div className="flex gap-2 flex-wrap">
                    {[
                      { slot: "hero", example: "image-cover", icon: "img" },
                      { slot: "badge", example: "badge-*", icon: "!" },
                      { slot: "title", example: "h1-h4", icon: "Aa" },
                      { slot: "primary", example: "tag, caption", icon: "1" },
                      { slot: "price", example: "price-lg", icon: "$" },
                      { slot: "secondary", example: "body-sm", icon: "2" },
                      { slot: "gallery", example: "gallery", icon: "GG" },
                      { slot: "stock", example: "badge", icon: "ok" },
                      { slot: "description", example: "body", icon: "Tt" },
                      { slot: "tags", example: "tag", icon: "#" },
                      { slot: "specs", example: "body-sm", icon: "Sp" },
                    ].map((a) => (
                      <div
                        key={a.slot}
                        className="px-2 py-1.5 rounded-lg text-xs border flex flex-col items-center min-w-16"
                        style={{ borderColor: "#E2E8F0" }}
                      >
                        <span className="text-lg">{a.icon}</span>
                        <span className="font-semibold">{a.slot}</span>
                        <span className="text-gray-400">{a.example}</span>
                      </div>
                    ))}
                  </div>
                  <div className="mt-3 pt-3 border-t" style={{ borderColor: "#E2E8F0" }}>
                    <div className="text-xs font-semibold text-gray-500 mb-1.5">Стили-алиасы (готовые наборы slot→display)</div>
                    <div className="flex gap-2 flex-wrap">
                      {[
                        { name: "product-hero", desc: "title→h1, price→price-lg, hero→image-cover" },
                        { name: "product-compact", desc: "title→h3, price→price, hero→thumbnail" },
                        { name: "product-detail", desc: "title→h1, gallery→gallery, description→body" },
                        { name: "service-card", desc: "title→h2, price→price, hero→image-cover" },
                        { name: "service-detail", desc: "title→h1, gallery→gallery, specs→body-sm" },
                      ].map((s) => (
                        <div
                          key={s.name}
                          className="px-2 py-1 rounded border text-xs"
                          style={{ borderColor: COLORS.agent2.border + "44", background: COLORS.agent2.bg }}
                          title={s.desc}
                        >
                          <span className="font-semibold" style={{ color: COLORS.agent2.text }}>{s.name}</span>
                        </div>
                      ))}
                    </div>
                  </div>
                </div>

                <Arrow label="атомы группируются в" />

                {/* Widget */}
                <div
                  className="rounded-xl p-4 w-full border"
                  style={{ borderColor: COLORS.agent2.border, background: COLORS.agent2.bg + "44" }}
                >
                  <div className="text-sm font-bold mb-2" style={{ color: COLORS.agent2.text }}>
                    Widget — карточка из атомов
                  </div>
                  <div className="flex gap-3 flex-wrap">
                    {["ProductCard", "ProductDetail", "ProductCompact", "ServiceCard"].map((t) => (
                      <div
                        key={t}
                        className="px-3 py-2 rounded-lg bg-white border text-sm"
                        style={{ borderColor: COLORS.agent2.border + "66" }}
                      >
                        {t}
                      </div>
                    ))}
                  </div>
                </div>

                <Arrow label="виджеты раскладываются в" />

                {/* Formation */}
                <div
                  className="rounded-xl p-4 w-full border"
                  style={{ borderColor: COLORS.frontend.border, background: COLORS.frontend.bg }}
                >
                  <div className="text-sm font-bold mb-2" style={{ color: COLORS.frontend.text }}>
                    Formation — раскладка
                  </div>
                  <div className="flex gap-3">
                    {[
                      { mode: "grid", desc: "сетка 3×4" },
                      { mode: "list", desc: "список" },
                      { mode: "carousel", desc: "карусель" },
                      { mode: "single", desc: "одна карточка" },
                    ].map((f) => (
                      <div
                        key={f.mode}
                        className="px-3 py-2 rounded-lg bg-white border text-sm text-center"
                        style={{ borderColor: COLORS.frontend.border + "66" }}
                      >
                        <div className="font-semibold">{f.mode}</div>
                        <div className="text-xs text-gray-500">{f.desc}</div>
                      </div>
                    ))}
                  </div>
                </div>

                <Arrow label="рендерится на экране" />

                <div
                  className="rounded-xl p-3 border text-center w-full text-sm font-medium"
                  style={{ borderColor: COLORS.user.border, background: COLORS.user.bg, color: COLORS.user.text }}
                >
                  Пользователь видит сетку карточек товаров
                </div>
              </div>
            )}

            {/* ===== SCENARIOS TAB ===== */}
            {activeTab === "scenarios" && (
              <div>
                <div className="flex gap-2 mb-4">
                  {scenarios.map((s) => (
                    <button
                      key={s.id}
                      onClick={() => setActiveScenario(s.id)}
                      className={`px-3 py-1.5 rounded-lg text-sm font-medium cursor-pointer transition-all ${
                        activeScenario === s.id ? "shadow-sm" : ""
                      }`}
                      style={{
                        background: activeScenario === s.id ? COLORS.agent1.border : "#F1F5F9",
                        color: activeScenario === s.id ? "#fff" : "#64748B",
                      }}
                    >
                      {s.title}
                    </button>
                  ))}
                </div>
                <ScenarioView
                  scenario={scenarios.find((s) => s.id === activeScenario)}
                />
              </div>
            )}

            {/* ===== STATE TAB ===== */}
            {activeTab === "state" && (
              <div className="space-y-3">
                <p className="text-sm text-gray-500 mb-2">
                  SessionState — вся информация о сессии пользователя
                </p>

                {[
                  {
                    zone: "Data",
                    color: COLORS.agent1,
                    desc: "Данные из каталога",
                    content: "products: [...12 товаров], services: []",
                    writer: "Agent1 через search_products",
                  },
                  {
                    zone: "Meta",
                    color: COLORS.agent1,
                    desc: "Метаинформация о данных",
                    content: 'productCount: 12, fields: ["name","price","rating"], aliases: {tenant: "nike"}',
                    writer: "Agent1 (автоматически при UpdateData)",
                  },
                  {
                    zone: "Template",
                    color: COLORS.agent2,
                    desc: "Как отображать данные",
                    content: 'Formation { mode: "grid", widgets: [...12 виджетов с атомами] }',
                    writer: "Agent2 через render_preset или freestyle",
                  },
                  {
                    zone: "View",
                    color: COLORS.state,
                    desc: "Текущий режим просмотра",
                    content: 'mode: "grid" | "detail", focused: null | {type: "product", id: "..."}',
                    writer: "Навигация (expand/back)",
                  },
                  {
                    zone: "ViewStack",
                    color: COLORS.state,
                    desc: "История навигации (для кнопки Назад)",
                    content: "[{mode: grid, step: 5}, {mode: detail, step: 8}]",
                    writer: "PushView при expand, PopView при back",
                  },
                  {
                    zone: "ConversationHistory",
                    color: COLORS.llm,
                    desc: "История разговора с LLM",
                    content: '[{role:"user", "покажи Nike"}, {role:"assistant", toolCall:...}, ...]',
                    writer: "Agent1 после каждого запроса (для контекста и кэширования)",
                  },
                ].map((z) => (
                  <div
                    key={z.zone}
                    className="rounded-xl p-4 border-l-4"
                    style={{
                      borderColor: z.color.border,
                      background: z.color.bg,
                    }}
                  >
                    <div className="flex justify-between items-start">
                      <div>
                        <span
                          className="font-bold text-sm"
                          style={{ color: z.color.text }}
                        >
                          {z.zone}
                        </span>
                        <span className="text-xs text-gray-500 ml-2">
                          {z.desc}
                        </span>
                      </div>
                    </div>
                    <div
                      className="text-xs font-mono mt-1.5 p-2 rounded"
                      style={{ background: "#fff", color: "#475569" }}
                    >
                      {z.content}
                    </div>
                    <div className="text-xs mt-1.5" style={{ color: "#94A3B8" }}>
                      Кто пишет: {z.writer}
                    </div>
                  </div>
                ))}

                <div
                  className="rounded-xl p-4 border mt-4 cursor-pointer"
                  style={{ borderColor: COLORS.state.border, background: COLORS.state.bg }}
                  onClick={() => handleClick("delta")}
                >
                  <div className="font-bold text-sm" style={{ color: COLORS.state.text }}>
                    Delta (Event Sourcing)
                  </div>
                  <p className="text-xs text-gray-600 mt-1">
                    Каждое изменение любой зоны создаёт Delta — запись "что изменилось, кем, когда".
                    Это позволяет откатиться к любому шагу и воспроизвести историю.
                  </p>
                  <div className="text-xs font-mono mt-2 p-2 rounded bg-white" style={{ color: "#475569" }}>
                    step:3 → agent1 → search_products → data.products (добавил 12 товаров)
                    <br />
                    step:4 → agent2 → render_product_preset → template.formation (grid 3x4)
                  </div>
                </div>
              </div>
            )}
          </div>

          {/* Detail panel */}
          <div className="lg:col-span-2">
            {selected ? (
              <div className="sticky top-4">
                <DetailPanel
                  detail={selected}
                  onClose={() => setSelected(null)}
                />
              </div>
            ) : (
              <div
                className="rounded-xl p-5 border text-center"
                style={{ background: "#FAFBFC", borderColor: "#E2E8F0" }}
              >
                <div className="text-4xl mb-3 opacity-30">&#8592;</div>
                <p className="text-sm" style={{ color: "#94A3B8" }}>
                  Кликни на любой элемент слева чтобы увидеть подробности
                </p>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
