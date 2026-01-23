# Core Design Principles

## Design Process

### 1. Discover
- User research: interviews, surveys, analytics
- Competitive analysis
- Constraints: technical, business, timeline
- Success metrics definition

### 2. Define
- Clear problem statement
- User stories: who needs what and why
- Acceptance criteria
- Edge cases identification

### 3. Explore
- Sketches: low-fidelity, fast iteration
- Wireframes: structure without style
- Prototypes: interactive, testable
- Design reviews: feedback early

### 4. Validate
- Usability testing
- Accessibility testing
- Performance testing
- A/B testing when appropriate

### 5. Implement
- Design handoff with clear specs
- Component development
- Frontend code review for design/engineering alignment
- QA: visual, functional, accessibility testing

### 6. Iterate
- Monitor metrics
- Gather feedback
- Identify improvements
- Plan next iteration

## Token Architecture

```typescript
// Core tokens (primitive values)
export const primitives = {
  color: {
    blue: { 50: "#eff6ff", /* ... */ 900: "#1e3a8a" },
    gray: { /* ... */ },
  },
  spacing: {
    0: "0",
    1: "0.25rem",  // 4px
    2: "0.5rem",   // 8px
    4: "1rem",     // 16px
    6: "1.5rem",   // 24px
    8: "2rem",     // 32px
  },
};

// Semantic tokens (meaningful aliases)
export const tokens = {
  color: {
    brand: { primary: primitives.color.blue[600] },
    text: { primary: primitives.color.gray[900] },
    background: { primary: primitives.color.gray[50] },
  },
};
```

## Token Categories

1. **Color**: brand, text, background, border, status
2. **Spacing**: padding, margin, gap (4px base)
3. **Typography**: font size, weight, line height, letter spacing
4. **Shadow**: elevation levels
5. **Border**: width, radius, style
6. **Timing**: animation duration
7. **Easing**: animation timing functions
8. **Z-index**: semantic layer names

## Common Antipatterns

```typescript
// Using pixel values directly
<div style={{ padding: "16px" }}>  // Bad
<div style={{ padding: tokens.spacing[4] }}>  // Good

// Button-like divs
<div onClick={handleClick}>Click me</div>  // Bad
<button onClick={handleClick}>Click me</button>  // Good

// Missing alt text
<img src="chart.png" />  // Bad
<img src="chart.png" alt="Sales up 45% in Q4" />  // Good

// Color-only communication
<span className="text-red">Error</span>  // Bad
<span className="error"><AlertIcon /> Error: Invalid</span>  // Good
```

## Tool Stack

### Design
- Figma: primary design tool
- Contrast checkers: WebAIM, Stark
- Color palette generators: Coolors, ColorBox

### Development
- Design tokens: Style Dictionary
- Component library: React, TypeScript
- CSS: Tailwind with custom tokens
- Storybook: component documentation

### Testing
- Accessibility: axe DevTools, WAVE, Lighthouse
- Screen readers: NVDA, VoiceOver
- Performance: WebPageTest, Lighthouse
