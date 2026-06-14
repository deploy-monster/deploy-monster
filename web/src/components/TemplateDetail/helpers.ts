/** Parse service names from compose YAML */
export function parseServices(yaml: string): string[] {
  const services: string[] = [];
  const lines = yaml.split('\n');
  let inServices = false;
  for (const line of lines) {
    const trimmed = line.trim();
    if (trimmed === 'services:') {
      inServices = true;
      continue;
    }
    if (inServices) {
      const match = /^ {2}([a-zA-Z0-9_-]+):/.exec(line);
      if (match) {
        services.push(match[1]);
      } else if (!line.startsWith(' ') && trimmed !== '') {
        inServices = false;
      }
    }
  }
  return services;
}
