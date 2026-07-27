import Frame from './nodes/Frame'
import Group from './nodes/Group'
import Text from './nodes/Text'
import Image from './nodes/Image'
import Ref from './nodes/Ref'
import Rectangle from './nodes/Rectangle'
import Line from './nodes/Line'
import IconFont from './nodes/IconFont'
import Input from './nodes/Input'
import Select from './nodes/Select'
import DateInput from './nodes/DateInput'
import Textarea from './nodes/Textarea'
import Submit from './nodes/Submit'
import { FormProvider } from './form/FormContext'

// NodeRenderer — type dispatcher for V5 scene-graph nodes. Backend has
// already resolved refs (ResolveAndInline) and bound data (BindData)
// before the document reaches the frontend, so by the time we render:
//   frame / group → containers with children
//   text          → leaf with content + format + wrapper + textStyle
//   image         → leaf with fills[0].url or content URL
//   rectangle     → decorative leaf with solid fill / radius / stroke
//   line          → divider leaf (horizontal or vertical rule)
//   icon_font     → lucide icon leaf via iconFontName
//   ref           → opaque resolved subtree (treated transparently)
//   input / select / datetime / textarea / submit
//                 → form primitives (RUNTIME_SPEC §5.2); a frame
//                   carrying formId wraps its subtree in a FormProvider
//                   (values/errors/status live there, Frame untouched)
//
// Anything else logs a warn and renders nothing — diagnostic for new
// node types the renderer doesn't handle yet (ellipse, polygon, etc.).

export default function NodeRenderer({ node }) {
  if (!node || typeof node !== 'object') return null
  const t = node.type
  switch (t) {
    case 'frame':
      return node.formId ? (
        <FormProvider node={node}>
          <Frame node={node} />
        </FormProvider>
      ) : (
        <Frame node={node} />
      )
    case 'group':
      return <Group node={node} />
    case 'text':
      return <Text node={node} />
    case 'image':
      return <Image node={node} />
    case 'ref':
      return <Ref node={node} />
    case 'rectangle':
      return <Rectangle node={node} />
    case 'line':
      return <Line node={node} />
    case 'icon_font':
      return <IconFont node={node} />
    case 'input':
      return <Input node={node} />
    case 'select':
      return <Select node={node} />
    case 'datetime':
      return <DateInput node={node} />
    case 'textarea':
      return <Textarea node={node} />
    case 'submit':
      return <Submit node={node} />
    default:
      // eslint-disable-next-line no-console
      console.warn('[v5-renderer] unknown node type', t, node?.id)
      return null
  }
}
