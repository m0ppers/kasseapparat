declare global {
  var serverState: {
    userData: {
      gravatarUrl: string;
      role: string;
      username: string;
      id: number;
    } | null;
    expiryDate: number | null;
  };
}

export {};
