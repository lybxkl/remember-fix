// Safari 15.0–15.2（macOS 12.0–12.2）缺少两个 ES2022 方法，编辑器依赖会直接调用。
type HasOwnTarget = Record<PropertyKey, unknown>

if (typeof (Object as { hasOwn?: unknown }).hasOwn !== 'function') {
  Object.defineProperty(Object, 'hasOwn', {
    configurable: true,
    writable: true,
    value: (target: HasOwnTarget, property: PropertyKey) =>
      Object.prototype.hasOwnProperty.call(target, property),
  })
}

if (typeof (Array.prototype as { findLast?: unknown }).findLast !== 'function') {
  Object.defineProperty(Array.prototype, 'findLast', {
    configurable: true,
    writable: true,
    value: function findLast<T>(
      this: T[],
      predicate: (value: T, index: number, array: T[]) => unknown,
      thisArg?: unknown,
    ): T | undefined {
      for (let i = this.length - 1; i >= 0; i -= 1) {
        if (predicate.call(thisArg, this[i], i, this)) return this[i]
      }
      return undefined
    },
  })
}
