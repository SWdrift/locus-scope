declare module "write-file-atomic" {
  import type { PathLike } from "node:fs";

  const writeFileAtomic: (filename: PathLike, data: string | Uint8Array, options?: object) => Promise<void>;

  export default writeFileAtomic;
}
