const API_URL = import.meta.env.VITE_BLOG_API_URL || '';

export async function fetchPosts() {
  if (!API_URL) return null;
  try {
    const res = await fetch(`${API_URL}/api/posts?status=published`);
    if (!res.ok) return null;
    return await res.json();
  } catch {
    return null;
  }
}

export async function fetchPost(slug) {
  if (!API_URL) return null;
  try {
    const res = await fetch(`${API_URL}/api/posts/${slug}`);
    if (!res.ok) return null;
    return await res.json();
  } catch {
    return null;
  }
}

// Convert API post (markdown content) to the structured format used by components
export function apiPostToLocal(post) {
  if (!post) return null;

  // Parse markdown content into sections
  const sections = [];
  const lines = (post.content || '').split('\n');
  let currentList = null;

  for (const line of lines) {
    if (currentList && !line.startsWith('- ')) {
      sections.push({ type: 'list', items: currentList });
      currentList = null;
    }

    if (line.startsWith('## ')) {
      sections.push({ type: 'heading', content: line.replace('## ', '') });
    } else if (line.startsWith('- ')) {
      if (!currentList) currentList = [];
      currentList.push(line.replace('- ', ''));
    } else if (line.trim() !== '') {
      sections.push({ type: 'paragraph', content: line });
    }
  }
  if (currentList) {
    sections.push({ type: 'list', items: currentList });
  }

  // Extract intro (first paragraph before any heading)
  let intro = '';
  const firstHeadingIdx = sections.findIndex((s) => s.type === 'heading');
  if (firstHeadingIdx > 0) {
    intro = sections.slice(0, firstHeadingIdx).map((s) => s.content).join(' ');
    sections.splice(0, firstHeadingIdx);
  } else if (sections.length > 0 && sections[0].type === 'paragraph') {
    intro = sections[0].content;
    sections.splice(0, 1);
  }

  const date = new Date(post.created_at);
  const dateStr = date.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
  const wordCount = (post.content || '').split(/\s+/).length;
  const readTime = `${Math.max(1, Math.ceil(wordCount / 200))} min read`;

  return {
    slug: post.slug,
    title: post.title,
    excerpt: post.excerpt,
    category: post.category,
    date: dateStr,
    readTime,
    coverImage: post.cover_image || 'linear-gradient(135deg, #4285F4 0%, #1A73E8 100%)',
    featured: post.id === 1,
    body: { intro, sections },
  };
}
