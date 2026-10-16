import { Namespace, SubjectSet, Context } from "@ory/keto-namespace-types"

class Actor implements Namespace {}

// Team membership grants nothing until Team#members is assigned to a scoped role/capability.
class Team implements Namespace {
  related: {
    organization: Organization[]
    members: Actor[]
  }
}
class Organization implements Namespace {
  related: {
    management_owner: Actor[]
    members: Actor[]
    reader: (Actor | SubjectSet<Team, "members">)[]
    commenter: (Actor | SubjectSet<Team, "members">)[]
    editor: (Actor | SubjectSet<Team, "members">)[]
    owner: (Actor | SubjectSet<Team, "members">)[]
    generate: (Actor | SubjectSet<Team, "members">)[]
    download: (Actor | SubjectSet<Team, "members">)[]
    source_read: (Actor | SubjectSet<Team, "members">)[]
    resource_read: (Actor | SubjectSet<Team, "members">)[]
  }

  permits = {
    can_manage_organization: (ctx: Context): boolean => this.related.management_owner.includes(ctx.subject),
    can_status: (ctx: Context): boolean => this.permits.isReader(ctx) || this.permits.can_generate(ctx),
    can_discover: (ctx: Context): boolean => this.related.members.includes(ctx.subject),
    isOwner: (ctx: Context): boolean =>
      this.related.owner.includes(ctx.subject),
    isEditor: (ctx: Context): boolean =>
      this.related.owner.includes(ctx.subject) ||
      this.related.editor.includes(ctx.subject),
    isCommenter: (ctx: Context): boolean =>
      this.related.owner.includes(ctx.subject) ||
      this.related.editor.includes(ctx.subject) ||
      this.related.commenter.includes(ctx.subject),
    isReader: (ctx: Context): boolean =>
      this.related.owner.includes(ctx.subject) ||
      this.related.editor.includes(ctx.subject) ||
      this.related.commenter.includes(ctx.subject) ||
      this.related.reader.includes(ctx.subject),
    can_generate: (ctx: Context): boolean =>
      this.permits.isEditor(ctx) ||
      this.related.generate.includes(ctx.subject),
    can_download: (ctx: Context): boolean =>
      this.permits.isReader(ctx) ||
      this.related.download.includes(ctx.subject),
    can_source_read: (ctx: Context): boolean =>
      this.permits.isReader(ctx) ||
      this.related.source_read.includes(ctx.subject),
    can_resource_read: (ctx: Context): boolean =>
      this.permits.isReader(ctx) ||
      this.related.resource_read.includes(ctx.subject),
  }
}

class Workspace implements Namespace {
  related: {
    parent: Organization[]
    reader: (Actor | SubjectSet<Team, "members">)[]
    commenter: (Actor | SubjectSet<Team, "members">)[]
    editor: (Actor | SubjectSet<Team, "members">)[]
    owner: (Actor | SubjectSet<Team, "members">)[]
    generate: (Actor | SubjectSet<Team, "members">)[]
    download: (Actor | SubjectSet<Team, "members">)[]
    source_read: (Actor | SubjectSet<Team, "members">)[]
    resource_read: (Actor | SubjectSet<Team, "members">)[]
  }

  permits = {
    can_status: (ctx: Context): boolean => this.permits.isReader(ctx) || this.permits.can_generate(ctx),
    can_discover: (ctx: Context): boolean => this.related.parent.traverse((p) => p.permits.can_discover(ctx)),
    isOwner: (ctx: Context): boolean =>
      this.related.owner.includes(ctx.subject) ||
      this.related.parent.traverse((p) => p.permits.isOwner(ctx)),
    isEditor: (ctx: Context): boolean =>
      this.related.owner.includes(ctx.subject) ||
      this.related.editor.includes(ctx.subject) ||
      this.related.parent.traverse((p) => p.permits.isEditor(ctx)),
    isCommenter: (ctx: Context): boolean =>
      this.related.owner.includes(ctx.subject) ||
      this.related.editor.includes(ctx.subject) ||
      this.related.commenter.includes(ctx.subject) ||
      this.related.parent.traverse((p) => p.permits.isCommenter(ctx)),
    isReader: (ctx: Context): boolean =>
      this.related.owner.includes(ctx.subject) ||
      this.related.editor.includes(ctx.subject) ||
      this.related.commenter.includes(ctx.subject) ||
      this.related.reader.includes(ctx.subject) ||
      this.related.parent.traverse((p) => p.permits.isReader(ctx)),
    can_generate: (ctx: Context): boolean =>
      this.permits.isEditor(ctx) ||
      this.related.generate.includes(ctx.subject) ||
      this.related.parent.traverse((p) => p.permits.can_generate(ctx)),
    can_download: (ctx: Context): boolean =>
      this.permits.isReader(ctx) ||
      this.related.download.includes(ctx.subject) ||
      this.related.parent.traverse((p) => p.permits.can_download(ctx)),
    can_source_read: (ctx: Context): boolean =>
      this.permits.isReader(ctx) ||
      this.related.source_read.includes(ctx.subject) ||
      this.related.parent.traverse((p) => p.permits.can_source_read(ctx)),
    can_resource_read: (ctx: Context): boolean =>
      this.permits.isReader(ctx) ||
      this.related.resource_read.includes(ctx.subject) ||
      this.related.parent.traverse((p) => p.permits.can_resource_read(ctx)),
  }
}

class Project implements Namespace {
  related: {
    parent: Workspace[]
    reader: (Actor | SubjectSet<Team, "members">)[]
    commenter: (Actor | SubjectSet<Team, "members">)[]
    editor: (Actor | SubjectSet<Team, "members">)[]
    owner: (Actor | SubjectSet<Team, "members">)[]
    generate: (Actor | SubjectSet<Team, "members">)[]
    download: (Actor | SubjectSet<Team, "members">)[]
    source_read: (Actor | SubjectSet<Team, "members">)[]
    resource_read: (Actor | SubjectSet<Team, "members">)[]
  }

  permits = {
    can_status: (ctx: Context): boolean => this.permits.isReader(ctx) || this.permits.can_generate(ctx),
    can_discover: (ctx: Context): boolean => this.permits.isReader(ctx),
    isOwner: (ctx: Context): boolean =>
      this.related.owner.includes(ctx.subject) ||
      this.related.parent.traverse((p) => p.permits.isOwner(ctx)),
    isEditor: (ctx: Context): boolean =>
      this.related.owner.includes(ctx.subject) ||
      this.related.editor.includes(ctx.subject) ||
      this.related.parent.traverse((p) => p.permits.isEditor(ctx)),
    isCommenter: (ctx: Context): boolean =>
      this.related.owner.includes(ctx.subject) ||
      this.related.editor.includes(ctx.subject) ||
      this.related.commenter.includes(ctx.subject) ||
      this.related.parent.traverse((p) => p.permits.isCommenter(ctx)),
    isReader: (ctx: Context): boolean =>
      this.related.owner.includes(ctx.subject) ||
      this.related.editor.includes(ctx.subject) ||
      this.related.commenter.includes(ctx.subject) ||
      this.related.reader.includes(ctx.subject) ||
      this.related.parent.traverse((p) => p.permits.isReader(ctx)),
    can_generate: (ctx: Context): boolean =>
      this.permits.isEditor(ctx) ||
      this.related.generate.includes(ctx.subject) ||
      this.related.parent.traverse((p) => p.permits.can_generate(ctx)),
    can_download: (ctx: Context): boolean =>
      this.permits.isReader(ctx) ||
      this.related.download.includes(ctx.subject) ||
      this.related.parent.traverse((p) => p.permits.can_download(ctx)),
    can_source_read: (ctx: Context): boolean =>
      this.permits.isReader(ctx) ||
      this.related.source_read.includes(ctx.subject) ||
      this.related.parent.traverse((p) => p.permits.can_source_read(ctx)),
    can_resource_read: (ctx: Context): boolean =>
      this.permits.isReader(ctx) ||
      this.related.resource_read.includes(ctx.subject) ||
      this.related.parent.traverse((p) => p.permits.can_resource_read(ctx)),
  }
}


