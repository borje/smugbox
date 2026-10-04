import { useEffect, useRef, useState, type ImgHTMLAttributes } from 'react'
import { cn } from '@/lib/utils'

// Images revealed within STAGGER_WINDOW of each other start STAGGER_STEP apart,
// so a screenful of photos ripples in instead of fading in all at once.
const STAGGER_WINDOW = 250
const STAGGER_STEP = 80
const STAGGER_MAX = 900
let lastReveal = 0
let batchSize = 0

function nextDelay(): number {
  const now = performance.now()
  if (now - lastReveal > STAGGER_WINDOW) batchSize = 0
  lastReveal = now
  return Math.min(batchSize++ * STAGGER_STEP, STAGGER_MAX)
}

// A lazy <img> that stays invisible until it has loaded AND scrolled into
// view, then plays the theme's --reveal-animation (index.css: animate-reveal).
// Images the browser preloads below the fold therefore still reveal when you
// reach them.
export default function FadeImage({ className, style, ...props }: ImgHTMLAttributes<HTMLImageElement>) {
  const ref = useRef<HTMLImageElement>(null)
  const [loaded, setLoaded] = useState(false)
  // null until scrolled into view; then the stagger delay in ms.
  const [delay, setDelay] = useState<number | null>(typeof IntersectionObserver === 'undefined' ? 0 : null)

  useEffect(() => {
    const el = ref.current
    if (!el) return
    if (el.complete && el.naturalWidth) setLoaded(true)
    if (typeof IntersectionObserver === 'undefined') return
    const io = new IntersectionObserver(
      ([entry]) => {
        if (entry.isIntersecting) {
          setDelay(nextDelay())
          io.disconnect()
        }
      },
      { rootMargin: '0px 0px -5% 0px' },
    )
    io.observe(el)
    return () => io.disconnect()
  }, [])

  const shown = loaded && delay !== null
  return (
    <img
      loading="lazy"
      {...props}
      ref={ref}
      onLoad={() => setLoaded(true)}
      style={shown ? { ...style, animationDelay: `${delay}ms` } : style}
      className={cn(className, shown ? 'animate-reveal' : 'opacity-0')}
    />
  )
}
