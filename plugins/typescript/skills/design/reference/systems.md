# Design Systems

## Architecture Layers

### Layer 1: Tokens (Atomic Values)

```typescript
// Primitives - raw values
export const primitives = {
  color: { blue: { 50: "#eff6ff", 600: "#2563eb" } },
  spacing: { 4: "1rem", 6: "1.5rem" },
  borderRadius: { md: "0.375rem", lg: "0.5rem" },
};

// Semantic - meaningful aliases
export const tokens = {
  color: {
    brand: { primary: primitives.color.blue[600] },
    text: { primary: primitives.color.gray[900] },
  },
  zIndex: {
    dropdown: 1000,
    modal: 1200,
    tooltip: 1500,
  },
};
```

### Layer 2: Primitives (Basic Components)

```typescript
// Box - low-level layout primitive
interface BoxProps {
  as?: React.ElementType;
  padding?: keyof typeof primitives.spacing;
  children: React.ReactNode;
}
```

### Layer 3: Components (UI Building Blocks)

```typescript
interface ButtonProps {
  variant?: "primary" | "secondary" | "danger" | "ghost";
  size?: "sm" | "md" | "lg";
  disabled?: boolean;
  loading?: boolean;
  children: React.ReactNode;
}
```

### Layer 4: Patterns (Complex Compositions)

```typescript
// ProfileCard - composed from multiple components
function ProfileCard({ user }: { user: User }) {
  return (
    <Card>
      <Avatar src={user.avatar} />
      <Text weight="semibold">{user.name}</Text>
      <Button variant="primary">Follow</Button>
    </Card>
  );
}
```

## Component API Design

### Props Philosophy

Make the simple easy, the complex possible.

```typescript
// Simple default
<Button>Click Me</Button>

// Complex when needed
<Button variant="danger" size="lg" loading>Delete</Button>
```

### Variant Pattern

Use variants for visual styles only.

```typescript
variant?: "primary" | "secondary" | "danger" | "ghost"
```

### Size Pattern

Consistent sizing across components.

```typescript
size?: "sm" | "md" | "lg"
```

### Composition Pattern

```typescript
// Composable
<Card>
  <CardHeader><CardTitle>Title</CardTitle></CardHeader>
  <CardContent>Content</CardContent>
  <CardFooter><Button>Action</Button></CardFooter>
</Card>

// Not monolithic
<Card title="Title" content="Content" buttons={[...]} />
```

## Versioning

### Semantic Versioning

- **Major (X.0.0)**: Breaking changes
- **Minor (0.X.0)**: New features, deprecations
- **Patch (0.0.X)**: Bug fixes

### Changelog Template

```markdown
## [2.1.0] - 2025-01-10

### Added
- New `Toast` component

### Changed
- Improved focus indicators

### Deprecated
- `Button` `block` prop

### Fixed
- `Modal` focus trap
```

## Governance

### Contribution Process

1. RFC proposal
2. Design review
3. Implementation with tests/docs
4. Code review
5. Accessibility review
6. Versioned release

### Decision Framework

- **Must Have**: Used in 3+ products
- **Should Have**: Useful but not critical
- **Nice to Have**: Edge cases
- **Won't Have**: Too specific

## Testing

### Unit Tests

```typescript
it("handles click events", () => {
  const handleClick = vi.fn();
  render(<Button onClick={handleClick}>Click</Button>);
  fireEvent.click(screen.getByRole("button"));
  expect(handleClick).toHaveBeenCalledOnce();
});
```

### Accessibility Tests

```typescript
it("has no accessibility violations", async () => {
  const { container } = render(<Button>Click</Button>);
  const results = await axe(container);
  expect(results).toHaveNoViolations();
});
```

### Visual Regression

```typescript
test("button variants match snapshots", async ({ page }) => {
  await page.goto("/iframe.html?id=components-button--all-variants");
  await expect(page).toHaveScreenshot("button-variants.png");
});
```

## Documentation Template

```markdown
# ComponentName

## Purpose
What this component does and when to use it.

## Usage
Basic code example.

## Variants
Available visual styles.

## Props
| Prop | Type | Default | Description |

## Accessibility
Keyboard, screen reader, focus behavior.

## States
Default, hover, focus, disabled, loading, error.

## Related Components
Links to similar components.
```
