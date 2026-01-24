# TypeScript Debugging

Debug patterns and tools for TypeScript and Node.js.

## Console Methods

```typescript
// Basic logging
console.log('value:', value);
console.error('Error:', error);
console.warn('Warning:', message);

// Structured output
console.table(arrayOfObjects);
console.dir(complexObject, { depth: null });

// Timing
console.time('operation');
// ... operation ...
console.timeEnd('operation');

// Grouping related logs
console.group('Request Processing');
console.log('Step 1');
console.log('Step 2');
console.groupEnd();

// Conditional logging
console.assert(condition, 'Condition failed:', data);

// Stack trace without error
console.trace('How did we get here?');
```

## Debugger Statement

```typescript
function processOrder(order: Order): void {
  // Pauses execution when DevTools is open
  debugger;

  // Conditional debugger
  if (order.total > 10000) {
    debugger;
  }
}
```

## Source Maps

Ensure `tsconfig.json` has source maps enabled:

```json
{
  "compilerOptions": {
    "sourceMap": true,
    "inlineSources": true
  }
}
```

For debugging bundled code, verify source maps are generated and served:

```typescript
// webpack.config.js
module.exports = {
  devtool: 'source-map', // Full source maps for debugging
};
```

## Node.js Debugging

```bash
# Start with inspector
node --inspect dist/server.js

# Break on first line
node --inspect-brk dist/server.js

# With ts-node
node --inspect -r ts-node/register src/server.ts
```

## Chrome DevTools Tips

1. **Breakpoints**: Click line number in Sources panel
2. **Conditional breakpoints**: Right-click line number, add condition
3. **Logpoints**: Right-click, add logpoint (logs without pausing)
4. **Watch expressions**: Add variables to Watch panel
5. **Call stack**: Navigate up/down the call stack
6. **Scope**: Inspect local, closure, and global variables

## Async Debugging

```typescript
// Capture async stack traces
Error.stackTraceLimit = 50;

// Label promises for debugging
const result = await Promise.race([
  fetchData().then(data => ({ source: 'fetch', data })),
  timeout(5000).then(() => ({ source: 'timeout', data: null }))
]);
console.log('Winner:', result.source);
```
