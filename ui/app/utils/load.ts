import { tracked } from '@glimmer/tracking';

// Tracked state of one request. Create it from a @cached getter so it is
// replaced whenever the inputs change; a superseded request is never read.
export default class Load<T> {
  @tracked private result?: { value?: T; error?: string };
  private previous?: Load<T>;

  // `previous` keeps the last value on screen until this request resolves.
  constructor(promise: Promise<T>, previous?: Load<T>) {
    this.previous = previous;
    promise.then(
      (value) => {
        this.previous = undefined;
        this.result = { value };
      },
      (e: unknown) => {
        this.previous = undefined;
        this.result = { error: e instanceof Error ? e.message : String(e) };
      },
    );
  }

  get pending(): boolean {
    return !this.result;
  }

  get value(): T | undefined {
    return this.result ? this.result.value : this.previous?.value;
  }

  get error(): string {
    return this.result?.error ?? '';
  }
}
