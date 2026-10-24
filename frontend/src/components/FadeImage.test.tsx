import { fireEvent, render, screen } from '@testing-library/react'
import FadeImage from '@/components/FadeImage'

// jsdom has no IntersectionObserver, so images count as in view at once.
describe('FadeImage', () => {
  it('stays hidden until loaded', () => {
    render(<FadeImage src="/x.jpg" alt="x" />)
    const img = screen.getByAltText('x')
    expect(img).toHaveClass('opacity-0')
    fireEvent.load(img)
    expect(img).toHaveClass('animate-reveal')
  })

  it('reveals an image that failed to load', () => {
    render(<FadeImage src="/missing.jpg" alt="missing" />)
    const img = screen.getByAltText('missing')
    fireEvent.error(img)
    expect(img).toHaveClass('animate-reveal')
  })
})
