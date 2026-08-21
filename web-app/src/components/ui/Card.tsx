import {
  card,
  cardHeader,
  cardTitle,
  cardDescription,
  cardFooter,
  type CardVariants,
} from "./Card.css";

type CardProps = React.HTMLAttributes<HTMLDivElement> & CardVariants;

export function Card({ variant, padding, ...props }: CardProps) {
  return <div className={card({ variant, padding })} {...props} />;
}

export function CardHeader(props: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cardHeader} {...props} />;
}

export function CardTitle(props: React.HTMLAttributes<HTMLHeadingElement>) {
  return <h3 className={cardTitle} {...props} />;
}

export function CardDescription(props: React.HTMLAttributes<HTMLParagraphElement>) {
  return <p className={cardDescription} {...props} />;
}

export function CardFooter(props: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cardFooter} {...props} />;
}
