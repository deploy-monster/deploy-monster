export function getInitials(name: string) {
  return name
    .split(' ')
    .map((n) => n[0])
    .join('')
    .toUpperCase()
    .slice(0, 2);
}

export function formatDate(date: string) {
  return new Intl.DateTimeFormat(undefined, {
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  }).format(new Date(date));
}

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