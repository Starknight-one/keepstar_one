import { useState } from 'react'
import { ChatPanel } from './features/chat/ChatPanel'
import './App.css'

function App() {
  const [isChatOpen, setIsChatOpen] = useState(false)

  return (
    <div className="app">
      <main className="content">
        <h1>Welcome to Our Platform</h1>

        <section className="text-block">
          <p>
            Lorem ipsum dolor sit amet, consectetur adipiscing elit. Sed do eiusmod
            tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam,
            quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat.
          </p>
          <p>
            Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore
            eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident,
            sunt in culpa qui officia deserunt mollit anim id est laborum.
          </p>
        </section>

        <section className="text-block offset-right">
          <h2>Our Mission</h2>
          <p>
            Curabitur pretium tincidunt lacus. Nulla gravida orci a odio. Nullam varius,
            turpis et commodo pharetra, est eros bibendum elit, nec luctus magna felis
            sollicitudin mauris. Integer in mauris eu nibh euismod gravida.
          </p>
        </section>

        <section className="text-block">
          <p>
            Pellentesque habitant morbi tristique senectus et netus et malesuada fames
            ac turpis egestas. Proin pharetra nonummy pede. Mauris et orci. Aenean nec
            lorem. In porttitor. Donec laoreet nonummy augue.
          </p>
          <p>
            Suspendisse dui purus, scelerisque at, vulputate vitae, pretium mattis, nunc.
            Mauris eget neque at sem venenatis eleifend. Ut nonummy.
          </p>
        </section>

        <section className="text-block offset-left">
          <h2>What We Do</h2>
          <p>
            Fusce aliquet pede non pede. Suspendisse dapibus lorem pellentesque magna.
            Integer nulla. Donec blandit feugiat ligula. Donec hendrerit, felis et imperdiet
            euismod, purus ipsum pretium metus, in lacinia nulla nisl eget sapien.
          </p>
        </section>

        <section className="text-block">
          <p>
            Donec ut est in lectus consequat consequat. Etiam eget dui. Aliquam erat volutpat.
            Sed at lorem in nunc porta tristique. Proin nec augue. Quisque aliquam tempor magna.
          </p>
          <p>
            Pellentesque habitant morbi tristique senectus et netus et malesuada fames
            ac turpis egestas. Nunc ac magna. Maecenas odio dolor, vulputate vel, auctor ac,
            accumsan id, felis.
          </p>
        </section>

        <section className="text-block offset-right">
          <h2>Join Us</h2>
          <p>
            Morbi in sem quis dui placerat ornare. Pellentesque odio nisi, euismod in,
            pharetra a, ultricies in, diam. Sed arcu. Cras consequat. Praesent dapibus,
            neque id cursus faucibus, tortor neque egestas augue.
          </p>
          <p>
            Curabitur vulputate vestibulum lorem. Fusce sagittis, libero non molestie mollis,
            magna orci ultrices dolor, at vulputate neque nulla lacinia eros.
          </p>
        </section>
      </main>

      <button
        className="chat-toggle-btn"
        onClick={() => setIsChatOpen(!isChatOpen)}
      >
        {isChatOpen ? '✕' : '💬'}
      </button>

      {isChatOpen && <ChatPanel onClose={() => setIsChatOpen(false)} />}
    </div>
  )
}

export default App
