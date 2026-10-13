import { z } from "zod";

export const protocolVersion = 3 as const;

export const packageDescriptorSchema = z
  .object({
    root: z.string(),
    entry: z.string(),
    dependencies: z.record(z.string(), z.string())
  })
  .strict();

export const environmentSchema = z
  .object({
    rootScope: z.string(),
    rootDependencies: z.record(z.string(), z.string()),
    packages: z.record(z.string(), packageDescriptorSchema)
  })
  .strict();

export const requestSchema = z
  .object({
    version: z.literal(protocolVersion),
    workingDirectory: z.string(),
    arguments: z.array(z.string()),
    environment: environmentSchema,
    stdin: z.string().optional()
  })
  .strict();

export const responseSchema = z
  .object({
    version: z.literal(protocolVersion),
    exitCode: z.number().int().min(0).max(255),
    stdout: z.string(),
    stderr: z.string()
  })
  .strict();
export function decodeRequest(source: string): CliRequest {
  return parseRequest(JSON.parse(source));
}

export function encodeRequest(request: CliRequest): string {
  return `${JSON.stringify(request)}\n`;
}

export type PackageDescriptor = z.infer<typeof packageDescriptorSchema>;
export type Environment = z.infer<typeof environmentSchema>;
export type CliRequest = z.infer<typeof requestSchema>;
export type CliResponse = z.infer<typeof responseSchema>;

export function parseRequest(value: unknown): CliRequest {
  return requestSchema.parse(value);
}

export function parseResponse(value: unknown): CliResponse {
  return responseSchema.parse(value);
}

export function encodeResponse(response: CliResponse): string {
  return `${JSON.stringify(response)}\n`;
}
